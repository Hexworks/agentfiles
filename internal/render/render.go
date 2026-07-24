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

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/surfaces"
	"github.com/hexworks/agentfiles/internal/utils"
)

// RenderedFile is the final unit produced by the render pipeline: one target
// path plus the bytes that should exist there.
type RenderedFile struct {
	Path string
	Body []byte
	Mode os.FileMode
	// AssetID stores the asset id that produced this file, which makes previews
	// and future debugging easier.
	AssetID string
	// SourceRel is the forward-slash asset-relative path of the source
	// file inside the producing asset directory. Adopt uses it as the
	// reverse-mapping key: writing the local repo body back to
	// <asset.Dir>/<SourceRel> replaces the source content the render
	// pipeline read. See ADR 0020.
	SourceRel string
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
func Build(p *profile.Profile, proj *project.Manifest) (*ProjectPlan, []errs.DomainError) {
	// Step 1: turn the project's selected asset ids into loaded asset objects.
	// Any id that does not exist in the profile is recorded as an error but
	// does not stop processing — we want to report every problem at once.
	selectedAssets, resolveErrors := resolveAssets(p, proj)

	var domainErrors []errs.DomainError
	domainErrors = append(domainErrors, resolveErrors...)

	// detect exclusive_group conflicts.
	exclusives := map[string][]string{}
	for _, selectedAsset := range selectedAssets {
		if selectedAsset.ExclusiveGroup == "" {
			// Asset is not part of any exclusive group — nothing to track.
			continue
		}
		exclusives[selectedAsset.ExclusiveGroup] = append(exclusives[selectedAsset.ExclusiveGroup], selectedAsset.ID)
	}

	// We need to keep the groups sored to make output deterministic
	sortedGroups := make([]string, 0, len(exclusives))
	for group := range exclusives {
		sortedGroups = append(sortedGroups, group)
	}
	slices.Sort(sortedGroups)

	for _, group := range sortedGroups {
		// Deduplicate in case the same asset id was listed twice; sort for stable error messages.
		conflicts := utils.DeduplicateAndSort(exclusives[group])
		if len(conflicts) > 1 {
			// More than one distinct asset claims this group => conflict.
			domainErrors = append(domainErrors, ExclusiveGroupConflictError{Group: group, AssetIDs: conflicts})
		}
	}

	renderedFileMap := map[string]RenderedFile{}
	for _, asset := range selectedAssets {
		assetErrs := addRenderedFilesFor(renderedFileMap, asset, proj.EnabledAgents)
		domainErrors = append(domainErrors, assetErrs...)
	}

	if len(domainErrors) > 0 {
		return nil, domainErrors
	}

	var renderedFiles []RenderedFile
	for _, file := range renderedFileMap {
		renderedFiles = append(renderedFiles, file)
	}
	// We sort by path to keep output deterministic
	slices.SortFunc(renderedFiles, func(a, b RenderedFile) int {
		return strings.Compare(a.Path, b.Path)
	})
	return &ProjectPlan{Files: renderedFiles}, nil
}

// resolveAssets turns the selected asset ids from the project manifest into
// the loaded asset objects from the profile. Missing ids are accumulated and
// returned together so the caller can list every missing selection at once.
func resolveAssets(profile *profile.Profile, proj *project.Manifest) ([]*asset.Asset, []errs.DomainError) {
	var selected []*asset.Asset
	var domainErrs []errs.DomainError
	for _, id := range proj.SelectedAssetIDs {
		asset := profile.Assets[id]
		if asset == nil {
			domainErrs = append(domainErrs, AssetNotFoundError{AssetID: id})
			continue
		}
		selected = append(selected, asset)
	}
	return selected, domainErrs
}

// addRenderedFilesFor handles the type-specific render rules. The three built-in
// special cases are:
//   - skill: different output shape per agent
//   - agents_doc: maps to AGENTS.md for Codex
//   - settings: uses well-known config file names per agent
//
// Everything else uses generic projections. Task 0011 tracks replacing
// this switch with a per-(Type, Agent) strategy lookup.
func addRenderedFilesFor(files map[string]RenderedFile, a *asset.Asset, enabledAgents []agent.Agent) []errs.DomainError {
	switch a.Type {
	case asset.TypeSkill:
		return addSkillOutputs(files, a, enabledAgents)
	case asset.TypeAgentsDoc:
		if !slices.Contains(enabledAgents, agent.Codex) {
			return nil
		}
		body, err := readAssetFile(a, config.AgentsDocStarterFileName, "read")
		if err != nil {
			return []errs.DomainError{err}
		}
		target := config.AgentsDocStarterFileName
		files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, AssetID: a.ID, SourceRel: config.AgentsDocStarterFileName}
		return nil
	case asset.TypeSettings:
		var domainErrs []errs.DomainError
		// Per-agent settings source/target conventions are owned by the agent
		// package (agent.Descriptors) so the recognized set and its render
		// conventions live in one place.
		for _, d := range agent.Descriptors() {
			if !slices.Contains(enabledAgents, d.Agent) || !asset.SupportsAgent(a, d.Agent) {
				continue
			}
			path := filepath.Join(a.Dir, d.SettingsSource)
			if !utils.Exists(path) {
				continue
			}
			body, err := readAssetFile(a, d.SettingsSource, "read")
			if err != nil {
				domainErrs = append(domainErrs, err)
				continue
			}
			files[d.SettingsTarget] = RenderedFile{Path: d.SettingsTarget, Body: body, Mode: 0o644, AssetID: a.ID, SourceRel: d.SettingsSource}
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
			files[projection.Target] = RenderedFile{Path: projection.Target, Body: body, Mode: 0o644, AssetID: a.ID, SourceRel: filepath.ToSlash(projection.Source)}
		}
		return domainErrs
	}
}

// addSkillOutputs expands a single skill asset into each enabled agent's
// expected directory or file structure. Per-agent container roots and
// the Cursor flat-file layout come from internal/surfaces so the same
// paths back both rendering and folder-registration eligibility.
func addSkillOutputs(files map[string]RenderedFile, a *asset.Asset, enabledAgents []agent.Agent) []errs.DomainError {
	body, err := readAssetFile(a, config.SkillStarterFileName, "read")
	if err != nil {
		return []errs.DomainError{err}
	}
	relFiles, listErr := asset.RelativeFiles(a.Dir)
	if listErr != nil {
		return []errs.DomainError{listErr}
	}
	var domainErrs []errs.DomainError
	for _, ag := range enabledAgents {
		if !asset.SupportsAgent(a, ag) {
			continue
		}
		if root, ok := surfaces.SkillRoot(ag.String()); ok {
			for _, rel := range relFiles {
				data, readErr := readAssetFile(a, rel, "read")
				if readErr != nil {
					domainErrs = append(domainErrs, readErr)
					continue
				}
				target := filepath.ToSlash(filepath.Join(root, a.ID, rel))
				files[target] = RenderedFile{Path: target, Body: data, Mode: 0o644, AssetID: a.ID, SourceRel: filepath.ToSlash(rel)}
			}
			continue
		}
		if ag == agent.Cursor {
			target := filepath.ToSlash(filepath.Join(surfaces.CursorCommandsRoot(), a.ID+".md"))
			files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, AssetID: a.ID, SourceRel: config.SkillStarterFileName}
		}
	}
	return domainErrs
}

// readAssetFile reads a file inside an asset directory and turns any I/O
// failure into a typed render error that carries only the asset-relative
// path (never the absolute filesystem path).
func readAssetFile(asset *asset.Asset, relativePath, op string) ([]byte, errs.DomainError) {
	body, err := os.ReadFile(filepath.Join(asset.Dir, relativePath))
	if err == nil {
		return body, nil
	}
	return nil, classifyFileError(asset.ID, relativePath, op, err)
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
func walkProjection(
	files map[string]RenderedFile,
	asset *asset.Asset,
	sourceRel,
	targetRel string,
) []errs.DomainError {
	var domainErrs []errs.DomainError
	source := filepath.Join(asset.Dir, sourceRel)
	walkErr := filepath.WalkDir(source, func(path string, dir os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dir.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(source, path)
		// should not happen, maybe produce error here instead?
		if relErr != nil {
			rel = filepath.Base(path)
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			relPath := filepath.ToSlash(filepath.Join(sourceRel, rel))
			domainErrs = append(domainErrs, classifyFileError(asset.ID, relPath, "read", readErr))
			return nil
		}
		target := filepath.ToSlash(filepath.Join(targetRel, rel))
		files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, AssetID: asset.ID, SourceRel: filepath.ToSlash(filepath.Join(sourceRel, rel))}
		return nil
	})
	if walkErr != nil {
		domainErrs = append(domainErrs, AssetReadError{AssetID: asset.ID, RelPath: sourceRel, Op: "walk", Err: walkErr})
	}
	return domainErrs
}
