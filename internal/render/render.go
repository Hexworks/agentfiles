package render

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/addamsson/agentfiles/internal/asset"
	"github.com/addamsson/agentfiles/internal/fsutil"
	"github.com/addamsson/agentfiles/internal/profile"
	"github.com/addamsson/agentfiles/internal/project"
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

// allowedPrefixes is the coarse-grained safety fence for rendering. Assets may
// only target paths that are recognized as managed LLM-tooling surfaces.
var allowedPrefixes = []string{
	"AGENTS.md",
	".claude/",
	".cursor/",
	".codex/",
	".opencode/",
	".mcp.json",
}

// Build resolves a project's selected assets into the concrete files that
// should appear in the repository. It does not read the current repo state and
// it does not write anything; that is sync's job.
func Build(p *profile.Loaded, proj *project.Manifest) (*ProjectPlan, error) {
	selected, err := resolveAssets(p, proj)
	if err != nil {
		return nil, err
	}
	groupSelections := map[string]string{}
	for _, a := range selected {
		if a.ExclusiveGroup == "" {
			continue
		}
		if current, exists := groupSelections[a.ExclusiveGroup]; exists && current != a.ID {
			return nil, fmt.Errorf("multiple assets selected in exclusive group %s", a.ExclusiveGroup)
		}
		groupSelections[a.ExclusiveGroup] = a.ID
	}

	files := map[string]RenderedFile{}
	for _, a := range selected {
		if err := addAssetOutputs(files, a, proj.EnabledAgents); err != nil {
			return nil, fmt.Errorf("%s: %w", a.ID, err)
		}
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

// resolveAssets turns the selected asset ids from the project manifest into the
// loaded asset objects from the profile.
func resolveAssets(p *profile.Loaded, proj *project.Manifest) ([]*asset.Asset, error) {
	var selected []*asset.Asset
	for _, id := range proj.SelectedAssetIDs {
		a := p.Assets[id]
		if a == nil {
			return nil, fmt.Errorf("selected asset not found: %s", id)
		}
		selected = append(selected, a)
	}
	return selected, nil
}

// addAssetOutputs handles the type-specific render rules. The three built-in
// special cases are:
//   - skill: different output shape per agent
//   - agents_doc: maps to AGENTS.md for Codex
//   - settings: uses well-known config file names per agent
//
// Everything else uses generic projections.
func addAssetOutputs(files map[string]RenderedFile, a *asset.Asset, enabledAgents []string) error {
	switch a.Type {
	case asset.TypeSkill:
		return addSkillOutputs(files, a, enabledAgents)
	case asset.TypeAgentsDoc:
		if !slices.Contains(enabledAgents, "codex") {
			return nil
		}
		body, err := os.ReadFile(filepath.Join(a.Dir, "AGENTS.md"))
		if err != nil {
			return err
		}
		files["AGENTS.md"] = RenderedFile{Path: "AGENTS.md", Body: body, Mode: 0o644, Source: a.ID}
		return nil
	case asset.TypeSettings:
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
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[mapping.Target] = RenderedFile{Path: mapping.Target, Body: body, Mode: 0o644, Source: a.ID}
		}
		return nil
	default:
		for _, projection := range a.Projections {
			if !slices.Contains(enabledAgents, projection.Agent) {
				continue
			}
			if !asset.SupportsAgent(a, projection.Agent) {
				continue
			}
			if !isAllowedTarget(projection.Target) {
				return fmt.Errorf("target outside managed surfaces: %s", projection.Target)
			}
			source := filepath.Join(a.Dir, projection.Source)
			info, err := os.Stat(source)
			if err != nil {
				return err
			}
			if info.IsDir() {
				err = filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					body, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					rel := fsutil.Rel(source, path)
					target := filepath.ToSlash(filepath.Join(projection.Target, rel))
					files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, Source: a.ID}
					return nil
				})
				if err != nil {
					return err
				}
				continue
			}
			body, err := os.ReadFile(source)
			if err != nil {
				return err
			}
			files[projection.Target] = RenderedFile{Path: projection.Target, Body: body, Mode: 0o644, Source: a.ID}
		}
		return nil
	}
}

// addSkillOutputs expands a single skill asset into each enabled agent's
// expected directory or file structure.
func addSkillOutputs(files map[string]RenderedFile, a *asset.Asset, enabledAgents []string) error {
	skillFile := filepath.Join(a.Dir, "SKILL.md")
	body, err := os.ReadFile(skillFile)
	if err != nil {
		return err
	}
	relFiles, err := asset.RelativeFiles(a.Dir)
	if err != nil {
		return err
	}
	for _, agent := range enabledAgents {
		if !asset.SupportsAgent(a, agent) {
			continue
		}
		switch agent {
		case "codex":
			for _, rel := range relFiles {
				path := filepath.Join(a.Dir, rel)
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				target := filepath.ToSlash(filepath.Join(".codex/skills", a.ID, rel))
				files[target] = RenderedFile{Path: target, Body: data, Mode: 0o644, Source: a.ID}
			}
		case "claude-code":
			for _, rel := range relFiles {
				path := filepath.Join(a.Dir, rel)
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				target := filepath.ToSlash(filepath.Join(".claude/skills", a.ID, rel))
				files[target] = RenderedFile{Path: target, Body: data, Mode: 0o644, Source: a.ID}
			}
		case "opencode":
			for _, rel := range relFiles {
				path := filepath.Join(a.Dir, rel)
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				target := filepath.ToSlash(filepath.Join(".opencode/skills", a.ID, rel))
				files[target] = RenderedFile{Path: target, Body: data, Mode: 0o644, Source: a.ID}
			}
		case "cursor":
			target := filepath.ToSlash(filepath.Join(".cursor/commands", a.ID+".md"))
			files[target] = RenderedFile{Path: target, Body: body, Mode: 0o644, Source: a.ID}
		}
	}
	return nil
}

// isAllowedTarget enforces the managed-surfaces rule at render time.
func isAllowedTarget(target string) bool {
	target = filepath.ToSlash(target)
	for _, prefix := range allowedPrefixes {
		if target == prefix || strings.HasPrefix(target, prefix) {
			return true
		}
	}
	return false
}
