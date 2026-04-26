// Package asset models the reusable content units (skills, agent docs,
// settings, MCP configs, rules, hooks) that a profile can contain. It owns the
// on-disk manifest format and the scaffolding logic used when a new asset is
// created.
package asset

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/addamsson/agentfiles/internal/config"
	"github.com/addamsson/agentfiles/internal/fsutil"
)

// Type identifies the category of an asset and selects the render rules that
// apply to it.
type Type string

// Supported asset types. Each value maps to a distinct render behavior in
// internal/render; new types require matching logic there.
const (
	// TypeSkill stores reusable skill directories that render differently per
	// agent. Some agents want a directory, Cursor wants a single markdown file.
	TypeSkill Type = "skill"
	// TypeAgentsDoc is a project-level file (like AGENTS.md, CLAUDE.md)
	TypeAgentsDoc Type = "agents_doc"
	// TypeSettings holds per-agent configuration files (claude-code.json,
	// codex.toml, etc.) that render into each agent's well-known config path.
	TypeSettings Type = "settings"
	// TypeMCP holds Model Context Protocol server configuration.
	TypeMCP Type = "mcp"
	// TypeRule holds agent rule files projected via generic projections.
	TypeRule Type = "rule"
	// TypeHook holds shell hooks that the agent harness invokes around tool
	// calls or lifecycle events.
	TypeHook Type = "hook"
)

// AllTypes returns every supported asset type in the order callers should
// iterate them (used by profile.Init to scaffold per-type subdirectories).
// Adding a new Type constant requires extending this slice.
func AllTypes() []Type {
	return []Type{
		TypeSkill,
		TypeAgentsDoc,
		TypeSettings,
		TypeMCP,
		TypeRule,
		TypeHook,
	}
}

// Projection describes a generic source-to-target mapping for an asset file.
// It is mainly used by the generic asset types whose behavior is not hard-coded
// like skill/agents_doc/settings.
type Projection struct {
	Agent  string `json:"agent"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Manifest is the declarative description of one reusable asset.
//
// The manifest answers:
//   - what kind of asset this is
//   - which agents can use it
//   - whether it conflicts with other assets
//   - how its files should be projected into a repo
type Manifest struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Type             Type         `json:"type"`
	Description      string       `json:"description,omitempty"`
	Tags             []string     `json:"tags,omitempty"`
	CompatibleAgents []string     `json:"compatible_agents,omitempty"`
	ExclusiveGroup   string       `json:"exclusive_group,omitempty"`
	Projections      []Projection `json:"projections,omitempty"`
}

// Asset combines the manifest with its resolved filesystem location.
type Asset struct {
	Manifest
	Dir string
}

// Validate checks only the domain-level shape of the manifest. It does not
// inspect agent-specific projection semantics.
func (m Manifest) Validate() error {
	if m.ID == "" || m.Name == "" {
		return fmt.Errorf("asset id and name are required")
	}
	switch m.Type {
	case TypeSkill, TypeAgentsDoc, TypeSettings, TypeMCP, TypeRule, TypeHook:
	default:
		// FIX: return error object instead of string (@see task#0005)
		return fmt.Errorf("unsupported asset type: %s", m.Type)
	}
	return nil
}

// Load reads and validates one asset directory.
func Load(dir string) (*Asset, error) {
	var manifest Manifest
	if err := fsutil.ReadJSON(filepath.Join(dir, config.AssetManifestFileName), &manifest); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &Asset{Manifest: manifest, Dir: dir}, nil
}

// Init scaffolds a new asset directory with a starter file layout that matches
// the chosen type.
func Init(root string, manifest Manifest) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)
	if err := fsutil.EnsureDir(dir); err != nil {
		return "", err
	}
	if err := fsutil.WriteJSON(filepath.Join(dir, config.AssetManifestFileName), manifest); err != nil {
		return "", err
	}

	switch manifest.Type {
	case TypeSkill:
		body := []byte("---\nname: " + manifest.Name + "\ndescription: " + manifest.Description + "\n---\n\nDescribe the skill here.\n")
		if err := os.WriteFile(filepath.Join(dir, config.SkillStarterFileName), body, 0o644); err != nil {
			return "", err
		}
	case TypeAgentsDoc:
		if err := os.WriteFile(filepath.Join(dir, config.AgentsDocStarterFileName), []byte("# "+manifest.Name+"\n"), 0o644); err != nil {
			return "", err
		}
	case TypeSettings:
		if err := os.WriteFile(filepath.Join(dir, config.SettingsStarterFileName), []byte("# codex settings\n"), 0o644); err != nil {
			return "", err
		}
	}
	// TypeMCP, TypeRule, TypeHook intentionally produce no starter file —
	// their content is user-authored and the directory is left ready for it.
	// Manifest.Validate above rejects unknown types, so no default branch is
	// needed.

	return dir, nil
}

// SupportsAgent implements the "empty compatible_agents means all agents"
// convention used across rendering.
func SupportsAgent(a *Asset, agent string) bool {
	if len(a.CompatibleAgents) == 0 {
		return true
	}
	return slices.Contains(a.CompatibleAgents, agent)
}

// RelativeFiles returns all non-hidden content files inside an asset directory.
// asset.json is intentionally excluded because it is metadata, not renderable
// content.
func RelativeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := fsutil.ToRelative(root, path)
		if rel == config.AssetManifestFileName || strings.HasPrefix(rel, ".") {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}
