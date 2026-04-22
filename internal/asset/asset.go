package asset

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/addamsson/agentfiles/internal/fsutil"
)

type Type string

const (
	// TypeSkill stores reusable skill directories that render differently per
	// agent. Some agents want a directory, Cursor wants a single markdown file.
	TypeSkill     Type = "skill"
	TypeAgentsDoc Type = "agents_doc"
	TypeSettings  Type = "settings"
	TypeMCP       Type = "mcp"
	TypeRule      Type = "rule"
	TypeHook      Type = "hook"
)

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
		return fmt.Errorf("unsupported asset type: %s", m.Type)
	}
	return nil
}

// Load reads and validates one asset directory.
func Load(dir string) (*Asset, error) {
	var manifest Manifest
	if err := fsutil.ReadJSON(filepath.Join(dir, "asset.json"), &manifest); err != nil {
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
	dir := filepath.Join(root, "assets", string(manifest.Type), manifest.ID)
	if err := fsutil.EnsureDir(dir); err != nil {
		return "", err
	}
	if err := fsutil.WriteJSON(filepath.Join(dir, "asset.json"), manifest); err != nil {
		return "", err
	}

	switch manifest.Type {
	case TypeSkill:
		body := []byte("---\nname: " + manifest.Name + "\ndescription: " + manifest.Description + "\n---\n\nDescribe the skill here.\n")
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), body, 0o644); err != nil {
			return "", err
		}
	case TypeAgentsDoc:
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# "+manifest.Name+"\n"), 0o644); err != nil {
			return "", err
		}
	case TypeSettings:
		if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte("# codex settings\n"), 0o644); err != nil {
			return "", err
		}
	default:
		if err := os.WriteFile(filepath.Join(dir, ".keep"), []byte{}, 0o644); err != nil {
			return "", err
		}
	}

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
		rel := fsutil.Rel(root, path)
		if rel == "asset.json" || strings.HasPrefix(rel, ".") {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}
