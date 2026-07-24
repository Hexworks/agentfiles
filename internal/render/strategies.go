package render

import (
	"os"
	"path/filepath"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/surfaces"
	"github.com/hexworks/agentfiles/internal/utils"
)

// suffixReverse is the shared reverse behavior for every non-lossy
// strategy. An exact rendered-file match returns the recorded SourceRel
// verbatim (it already captures any filename change the forward render
// applied); an unknown sibling file preserves its tail below the owning
// directory's target root. Embedding it keeps the four structural
// strategies from repeating the same inverse.
type suffixReverse struct{}

func (suffixReverse) Reverse(ctx ReverseContext) (string, bool) {
	if ctx.ExactSourceRel != "" {
		return ctx.ExactSourceRel, true
	}
	return reverseSuffixPreserving(ctx)
}

// reverseSuffixPreserving maps an unknown sibling repo path back to its
// asset-relative source by stripping the owning dir's target root and
// re-anchoring the tail under its source root. It is the exact inverse
// of the folder/dir-projection forward render and reproduces the tail
// arithmetic the sync engine used before the strategy table owned it.
func reverseSuffixPreserving(ctx ReverseContext) (string, bool) {
	var tail string
	switch {
	case ctx.TargetRoot == "." || ctx.TargetRoot == "":
		tail = ctx.RepoPath
	case ctx.RepoPath == ctx.TargetRoot:
		tail = ""
	case len(ctx.RepoPath) > len(ctx.TargetRoot) && ctx.RepoPath[:len(ctx.TargetRoot)+1] == ctx.TargetRoot+"/":
		tail = ctx.RepoPath[len(ctx.TargetRoot)+1:]
	default:
		return "", false
	}
	switch {
	case ctx.SourceRoot == "." || ctx.SourceRoot == "":
		return tail, true
	case tail == "":
		return ctx.SourceRoot, true
	default:
		return ctx.SourceRoot + "/" + tail, true
	}
}

// skillFolderStrategy renders a skill as one child folder per skill under
// the agent's container root (.codex/skills, .claude/skills,
// .opencode/skills), mirroring the asset directory's file layout. Reverse
// is suffix-preserving: the rendered dir mirrors the source dir, so an
// untracked sibling maps straight back.
type skillFolderStrategy struct {
	suffixReverse
	root string
}

func (s skillFolderStrategy) Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError) {
	// Validate the skill body up front so a skill with a manifest but no
	// SKILL.md fails with AssetSourceMissingError instead of rendering an
	// empty (bodyless) folder.
	if _, err := readAssetFile(a, config.SkillStarterFileName, "read"); err != nil {
		return nil, []errs.DomainError{err}
	}
	relFiles, listErr := asset.RelativeFiles(a.Dir)
	if listErr != nil {
		return nil, []errs.DomainError{listErr}
	}
	var files []RenderedFile
	var derrs []errs.DomainError
	for _, rel := range relFiles {
		data, readErr := readAssetFile(a, rel, "read")
		if readErr != nil {
			derrs = append(derrs, readErr)
			continue
		}
		target := filepath.ToSlash(filepath.Join(s.root, a.ID, rel))
		files = append(files, RenderedFile{
			Path:      target,
			Body:      data,
			Mode:      0o644,
			AssetID:   a.ID,
			SourceRel: filepath.ToSlash(rel),
			Agent:     ag,
			Type:      a.Type,
		})
	}
	return files, derrs
}

// cursorSkillStrategy flattens a whole skill into a single
// .cursor/commands/<id>.md file, dropping every sibling filename. That
// makes it intentionally lossy: Reverse reports ok=false, so a cursor
// command is never offered for Adopt.
type cursorSkillStrategy struct{}

func (cursorSkillStrategy) Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError) {
	body, err := readAssetFile(a, config.SkillStarterFileName, "read")
	if err != nil {
		return nil, []errs.DomainError{err}
	}
	target := filepath.ToSlash(filepath.Join(surfaces.CursorCommandsRoot(), a.ID+".md"))
	return []RenderedFile{{
		Path:      target,
		Body:      body,
		Mode:      0o644,
		AssetID:   a.ID,
		SourceRel: config.SkillStarterFileName,
		Agent:     ag,
		Type:      a.Type,
	}}, nil
}

// Reverse always fails: the flat file dropped the source layout, so there
// is no unambiguous asset path to write a local edit back into.
func (cursorSkillStrategy) Reverse(ReverseContext) (string, bool) {
	return "", false
}

// agentsDocStrategy renders the single agents_doc source file to a
// per-agent target: CLAUDE.md for claude-code, AGENTS.md for the others.
// The source filename stays AGENTS.md regardless, so Reverse of the
// renamed CLAUDE.md relies on the recorded SourceRel (suffixReverse's
// exact-match arm), not the target's tail.
type agentsDocStrategy struct {
	suffixReverse
	target string
}

func (s agentsDocStrategy) Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError) {
	body, err := readAssetFile(a, config.AgentsDocStarterFileName, "read")
	if err != nil {
		return nil, []errs.DomainError{err}
	}
	return []RenderedFile{{
		Path:      s.target,
		Body:      body,
		Mode:      0o644,
		AssetID:   a.ID,
		SourceRel: config.AgentsDocStarterFileName,
		Agent:     ag,
		Type:      a.Type,
	}}, nil
}

// settingsStrategy renders one agent's settings file from the source and
// target names its descriptor owns. A missing source file is not an
// error — the asset simply has nothing to project for this agent.
type settingsStrategy struct {
	suffixReverse
	descriptor agent.Descriptor
}

func (s settingsStrategy) Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError) {
	if !utils.Exists(filepath.Join(a.Dir, s.descriptor.SettingsSource)) {
		return nil, nil
	}
	body, err := readAssetFile(a, s.descriptor.SettingsSource, "read")
	if err != nil {
		return nil, []errs.DomainError{err}
	}
	return []RenderedFile{{
		Path:      s.descriptor.SettingsTarget,
		Body:      body,
		Mode:      0o644,
		AssetID:   a.ID,
		SourceRel: s.descriptor.SettingsSource,
		Agent:     ag,
		Type:      a.Type,
	}}, nil
}

// genericProjectionStrategy renders the projection-driven asset types
// (mcp, rule, hook) for one agent by walking a.Projections and emitting
// only those whose Agent matches. A projection whose Source is a
// directory is walked recursively, preserving relative layout. Reverse
// is suffix-preserving so an untracked sibling inside a walked directory
// maps back to its source.
type genericProjectionStrategy struct {
	suffixReverse
}

func (genericProjectionStrategy) Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError) {
	var files []RenderedFile
	var derrs []errs.DomainError
	for _, projection := range a.Projections {
		if projection.Agent != ag {
			continue
		}
		if !surfaces.IsAllowed(projection.Target) {
			derrs = append(derrs, TargetOutsideSurfacesError{AssetID: a.ID, Target: projection.Target})
			continue
		}
		source := filepath.Join(a.Dir, projection.Source)
		info, statErr := os.Stat(source)
		if statErr != nil {
			derrs = append(derrs, classifyFileError(a.ID, projection.Source, "stat", statErr))
			continue
		}
		if info.IsDir() {
			walked, walkErrs := walkProjectionFiles(a, ag, projection.Source, projection.Target)
			files = append(files, walked...)
			derrs = append(derrs, walkErrs...)
			continue
		}
		body, readErr := readAssetFile(a, projection.Source, "read")
		if readErr != nil {
			derrs = append(derrs, readErr)
			continue
		}
		files = append(files, RenderedFile{
			Path:      projection.Target,
			Body:      body,
			Mode:      0o644,
			AssetID:   a.ID,
			SourceRel: filepath.ToSlash(projection.Source),
			Agent:     ag,
			Type:      a.Type,
		})
	}
	return files, derrs
}
