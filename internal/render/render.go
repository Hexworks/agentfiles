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
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
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
	// Agent and Type record which (agent, asset-type) strategy produced
	// this file. They are provenance metadata kept for previews and
	// diagnostics (e.g. grouping files by agent); the reverse dispatch
	// uses Strategy directly, not these.
	Agent agent.Agent
	Type  asset.Type
	// Strategy is the render strategy that produced this file, stamped by
	// Build after Render returns. ReverseLookup dispatches Adopt's reverse
	// mapping off this handle, so forward and reverse resolve through the
	// same strategy instance without re-consulting the global table.
	Strategy Strategy
}

// ProjectPlan is the desired state of one project before sync compares it with
// the repo on disk.
type ProjectPlan struct {
	Files []RenderedFile
	// revIndex memoizes the reverse-lookup index built from Files on the
	// first ReverseLookup call. Populated lazily; the plan is read-only
	// and used single-threaded by sync, so no locking is needed.
	revIndex *reverseIndex
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
	for _, selectedAsset := range selectedAssets {
		assetErrs := addRenderedFilesFor(renderedFileMap, selectedAsset, proj.EnabledAgents)
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

// addRenderedFilesFor dispatches an asset to the per-(agent, type) render
// strategies. For every enabled agent the asset supports it looks up the
// strategy keyed by (agent, asset.Type) — a map lookup, never a switch —
// and merges the files that strategy renders into files. An enabled agent
// with no strategy for the asset's type accumulates an
// UnsupportedRenderingError rather than short-circuiting, so a project can
// still report every other problem in one pass.
func addRenderedFilesFor(files map[string]RenderedFile, a *asset.Asset, enabledAgents []agent.Agent) []errs.DomainError {
	var domainErrs []errs.DomainError
	for _, ag := range enabledAgents {
		if !asset.SupportsAgent(a, ag) {
			continue
		}
		strat, ok := strategyFor(ag, a.Type)
		if !ok {
			domainErrs = append(domainErrs, UnsupportedRenderingError{Agent: ag, Type: a.Type})
			continue
		}
		rendered, renderErrs := strat.Render(a, ag)
		domainErrs = append(domainErrs, renderErrs...)
		for _, file := range rendered {
			// Stamp the producing strategy so ReverseLookup dispatches
			// off it directly instead of re-resolving (agent, type)
			// through the global table.
			file.Strategy = strat
			files[file.Path] = file
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

// walkProjectionFiles projects every file under a directory source into
// the target tree, preserving relative layout, and returns them as
// RenderedFile values tagged with the producing agent and asset type.
// Shared by the generic-projection strategy across all agents.
func walkProjectionFiles(
	a *asset.Asset,
	ag agent.Agent,
	sourceRel,
	targetRel string,
) ([]RenderedFile, []errs.DomainError) {
	var files []RenderedFile
	var domainErrs []errs.DomainError
	source := filepath.Join(a.Dir, sourceRel)
	walkErr := filepath.WalkDir(source, func(path string, dir os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dir.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			// filepath.Rel only fails when path is not rooted under
			// source, which cannot happen inside a WalkDir rooted at
			// source. Treat it as a bug (errors.md exception #2): record
			// it and skip the file rather than flatten a nested path to
			// its bare name with filepath.Base, which would corrupt both
			// Path and SourceRel.
			domainErrs = append(domainErrs, classifyFileError(a.ID, sourceRel, "rel", relErr))
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			relPath := filepath.ToSlash(filepath.Join(sourceRel, rel))
			domainErrs = append(domainErrs, classifyFileError(a.ID, relPath, "read", readErr))
			return nil
		}
		target := filepath.ToSlash(filepath.Join(targetRel, rel))
		files = append(files, RenderedFile{
			Path:      target,
			Body:      body,
			Mode:      0o644,
			AssetID:   a.ID,
			SourceRel: filepath.ToSlash(filepath.Join(sourceRel, rel)),
			Agent:     ag,
			Type:      a.Type,
		})
		return nil
	})
	if walkErr != nil {
		domainErrs = append(domainErrs, AssetReadError{AssetID: a.ID, RelPath: sourceRel, Op: "walk", Err: walkErr})
	}
	return files, domainErrs
}
