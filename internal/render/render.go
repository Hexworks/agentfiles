// Package render turns a profile plus a project manifest into the concrete set
// of files that should exist in the target repository. It is read-only: it
// loads asset content, applies per-type render rules, and returns a plan. The
// actual filesystem writes live in internal/sync.
package render

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/fsutil"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/surfaces"
)

// RenderedFile is the final unit produced by the render pipeline: one target
// path plus the bytes that should exist there.
type RenderedFile struct {
	Path string
	Body []byte
	Mode os.FileMode
	// Source stores the asset id that produced this file, which makes previews
	// and future debugging easier.
	Source string
}

// ProjectPlan is the desired state of one project before sync compares it with
// the repo on disk.
type ProjectPlan struct {
	Files []RenderedFile
}

// Build resolves a project's selected assets into the concrete files that
// should appear in the repository. It does not read the current repo state and
// it does not write anything; that is sync's job.
//
// Errors are accumulated rather than short-circuited: every missing asset id,
// every exclusive_group conflict, and every per-asset render failure is
// returned together so the TUI can list them in one go.
func Build(p *profile.Profile, proj *project.Project) (*ProjectPlan, []errs.DomainError) {
	selected, resolveErrs := resolveAssets(p, proj)

	var domainErrs []errs.DomainError
	domainErrs = append(domainErrs, resolveErrs...)

	groupSelections := map[string][]string{}
	for _, a := range selected {
		if a.ExclusiveGroup == "" {
			continue
		}
		groupSelections[a.ExclusiveGroup] = append(groupSelections[a.ExclusiveGroup], a.ID)
	}
	groupKeys := make([]string, 0, len(groupSelections))
	for group := range groupSelections {
		groupKeys = append(groupKeys, group)
	}
	slices.Sort(groupKeys)
	for _, group := range groupKeys {
		unique := dedupSorted(groupSelections[group])
		if len(unique) > 1 {
			domainErrs = append(domainErrs, ExclusiveGroupConflictError{Group: group, AssetIDs: unique})
		}
	}

	files := map[string]RenderedFile{}
	for _, a := range selected {
		assetErrs := addAssetOutputs(files, a, proj.EnabledAgents)
		domainErrs = append(domainErrs, assetErrs...)
	}

	if len(domainErrs) > 0 {
		return nil, domainErrs
	}

	var rendered []RenderedFile
	for _, file := range files {
		rendered = append(rendered, file)
	}
	slices.SortFunc(rendered, func(a, b RenderedFile) int {
		return strings.Compare(a.Path, b.Path)
	})
	return &ProjectPlan{Files: rendered}, nil
}

// dedupSorted returns a sorted copy of ids with duplicates removed.
func dedupSorted(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// resolveAssets turns the selected asset ids from the project manifest into
// the loaded asset objects from the profile. Missing ids are accumulated and
// returned together so the caller can list every missing selection at once.
func resolveAssets(p *profile.Profile, proj *project.Project) ([]*asset.Asset, []errs.DomainError) {
	var selected []*asset.Asset
	var domainErrs []errs.DomainError
	for _, id := range proj.SelectedAssetIDs {
		a := p.Assets[id]
		if a == nil {
			domainErrs = append(domainErrs, AssetNotFoundError{AssetID: id})
			continue
		}
		selected = append(selected, a)
	}
	return selected, domainErrs
}

// addAssetOutputs handles the type-specific render rules. The three built-in
// special cases are:
//   - skill: different output shape per agent
//   - agents_doc: maps to AGENTS.md for Codex
//   - settings: uses well-known config file names per agent
//
// Everything else uses generic projections. Task 0011 tracks replacing
// this switch with a per-(Type, Agent) strategy lookup.
func addAssetOutputs(files map[string]RenderedFile, a *asset.Asset, enabledAgents []string) []errs.DomainError {
	switch a.Type {
	case asset.TypeSkill:
		return addSkillOutputs(files, a, enabledAgents)
	case asset.TypeAgentsDoc:
		if !slices.Contains(enabledAgents, "codex") {
			return nil
		}
		body, err := readAssetFile(a, config.AgentsDocStarterFileName, "read")
		if err != nil {
			return []errs.DomainError{err}
		}
		target := config.AgentsDocStarterFileName
		files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, Source: a.ID}
		return nil
	case asset.TypeSettings:
		var domainErrs []errs.DomainError
		for _, mapping := range []struct {
			Agent  string
			Source string
			Target string
		}{
			{"claude-code", "claude-code.json", ".claude/settings.local.json"},
			{"codex", "codex.toml", ".codex/config.toml"},
			{"cursor", "cursor.json", ".cursor/config.json"},
			{"opencode", "opencode.json", ".opencode/config.json"},
		} {
			if !slices.Contains(enabledAgents, mapping.Agent) || !asset.SupportsAgent(a, mapping.Agent) {
				continue
			}
			path := filepath.Join(a.Dir, mapping.Source)
			if !fsutil.Exists(path) {
				continue
			}
			body, err := readAssetFile(a, mapping.Source, "read")
			if err != nil {
				domainErrs = append(domainErrs, err)
				continue
			}
			files[mapping.Target] = RenderedFile{Path: mapping.Target, Body: body, Mode: 0o644, Source: a.ID}
		}
		return domainErrs
	default:
		var domainErrs []errs.DomainError
		for _, projection := range a.Projections {
			if !slices.Contains(enabledAgents, projection.Agent) {
				continue
			}
			if !asset.SupportsAgent(a, projection.Agent) {
				continue
			}
			if !surfaces.IsAllowed(projection.Target) {
				domainErrs = append(domainErrs, TargetOutsideSurfacesError{AssetID: a.ID, Target: projection.Target})
				continue
			}
			source := filepath.Join(a.Dir, projection.Source)
			info, err := os.Stat(source)
			if err != nil {
				domainErrs = append(domainErrs, classifyFileError(a.ID, projection.Source, "stat", err))
				continue
			}
			if info.IsDir() {
				walkErrs := walkProjection(files, a, projection.Source, projection.Target)
				domainErrs = append(domainErrs, walkErrs...)
				continue
			}
			body, readErr := readAssetFile(a, projection.Source, "read")
			if readErr != nil {
				domainErrs = append(domainErrs, readErr)
				continue
			}
			files[projection.Target] = RenderedFile{Path: projection.Target, Body: body, Mode: 0o644, Source: a.ID}
		}
		return domainErrs
	}
}

// addSkillOutputs expands a single skill asset into each enabled agent's
// expected directory or file structure.
func addSkillOutputs(files map[string]RenderedFile, a *asset.Asset, enabledAgents []string) []errs.DomainError {
	body, err := readAssetFile(a, config.SkillStarterFileName, "read")
	if err != nil {
		return []errs.DomainError{err}
	}
	relFiles, listErr := asset.RelativeFiles(a.Dir)
	if listErr != nil {
		return []errs.DomainError{listErr}
	}
	skillRoots := map[string]string{
		"codex":       ".codex/skills",
		"claude-code": ".claude/skills",
		"opencode":    ".opencode/skills",
	}
	var domainErrs []errs.DomainError
	for _, agent := range enabledAgents {
		if !asset.SupportsAgent(a, agent) {
			continue
		}
		if root, ok := skillRoots[agent]; ok {
			for _, rel := range relFiles {
				data, readErr := readAssetFile(a, rel, "read")
				if readErr != nil {
					domainErrs = append(domainErrs, readErr)
					continue
				}
				target := filepath.ToSlash(filepath.Join(root, a.ID, rel))
				files[target] = RenderedFile{Path: target, Body: data, Mode: 0o644, Source: a.ID}
			}
			continue
		}
		if agent == "cursor" {
			target := filepath.ToSlash(filepath.Join(".cursor/commands", a.ID+".md"))
			files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, Source: a.ID}
		}
	}
	return domainErrs
}

// readAssetFile reads a file inside an asset directory and turns any I/O
// failure into a typed render error that carries only the asset-relative
// path (never the absolute filesystem path).
func readAssetFile(a *asset.Asset, rel, op string) ([]byte, errs.DomainError) {
	body, err := os.ReadFile(filepath.Join(a.Dir, rel))
	if err == nil {
		return body, nil
	}
	return nil, classifyFileError(a.ID, rel, op, err)
}

// classifyFileError discriminates between "file not present" (typed as
// AssetSourceMissingError) and other I/O failures (AssetReadError).
func classifyFileError(assetID, rel, op string, err error) errs.DomainError {
	if errors.Is(err, fs.ErrNotExist) {
		return AssetSourceMissingError{AssetID: assetID, RelPath: rel}
	}
	return AssetReadError{AssetID: assetID, RelPath: rel, Op: op, Err: err}
}

// walkProjection projects every file under a directory source into the
// target tree, preserving relative layout.
func walkProjection(files map[string]RenderedFile, a *asset.Asset, sourceRel, targetRel string) []errs.DomainError {
	var domainErrs []errs.DomainError
	source := filepath.Join(a.Dir, sourceRel)
	walkErr := filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			rel = filepath.Base(path)
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			domainErrs = append(domainErrs, classifyFileError(a.ID, filepath.ToSlash(filepath.Join(sourceRel, rel)), "read", readErr))
			return nil
		}
		target := filepath.ToSlash(filepath.Join(targetRel, rel))
		files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, Source: a.ID}
		return nil
	})
	if walkErr != nil {
		domainErrs = append(domainErrs, AssetReadError{AssetID: a.ID, RelPath: sourceRel, Op: "walk", Err: walkErr})
	}
	return domainErrs
}
