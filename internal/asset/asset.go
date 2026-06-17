// Package asset models the reusable content units (skills, agent docs,
// settings, MCP configs, rules, hooks) that a profile can contain. It owns the
// on-disk manifest format and the scaffolding logic used when a new asset is
// created.
package asset

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
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
// This is (similar to Profile) what we save into asset.json
//
// The manifest answers:
//   - what kind of asset this is
//   - which agents can use it
//   - whether it conflicts with other assets
//   - how its files should be projected into a repo
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        Type   `json:"type"`
	Description string `json:"description,omitempty"`
	// Tags are free-form labels used by the TUI to group and filter assets
	// when the user picks which ones to attach to a project. They carry no
	// render semantics: the same set of files is produced regardless of tags.
	//
	// Example: []string{"review", "security"} on a rule asset lets the user
	// narrow the asset picker to security-related rules.
	Tags []string `json:"tags,omitempty"`
	// CompatibleAgents restricts which enabled agents the asset will render
	// for. Empty means "every enabled agent" (see SupportsAgent). Names must
	// match values in project.Manifest.EnabledAgents (codex, claude-code,
	// cursor, opencode). See task 0009 for typed-value follow-up.
	CompatibleAgents []string `json:"compatible_agents,omitempty"`
	// ExclusiveGroup marks the asset as a member of a mutually-exclusive set:
	// at most one selected asset per group may render for a given project.
	//
	// render.Build refuses to produce a plan when two selected assets share
	// the same non-empty group.
	//
	// Example: two agents_doc assets — "codex-default" and "codex-strict" —
	// both set ExclusiveGroup: "main-agents-doc" so a project cannot
	// accidentally select both.
	ExclusiveGroup string `json:"exclusive_group,omitempty"`
	// Projections is the generic source-to-target mapping used by asset types
	// without hard-coded render rules (mcp, rule, hook). For skill,
	// agents_doc, and settings, render derives targets from agent
	// conventions and ignores this field.
	//
	// Each Projection.Target must stay inside the managed surfaces fence
	// (see internal/surfaces); render rejects the plan otherwise. If Source
	// points to a directory, render walks it and projects every file under
	// Target preserving relative paths.
	//
	// Example for a hook asset:
	//   [{Agent: "claude-code", Source: "pre-tool.sh",
	//     Target: ".claude/hooks/pre-tool.sh"}]
	Projections []Projection `json:"projections,omitempty"`
}

// Asset combines the manifest with its resolved filesystem location.
type Asset struct {
	Manifest
	Dir string
}

// Validate checks only the domain-level shape of the manifest. It does not
// inspect agent-specific projection semantics.
func (m Manifest) Validate() errs.DomainError {
	if m.ID == "" || m.Name == "" {
		return ErrAssetIDNameRequired
	}
	switch m.Type {
	case TypeSkill, TypeAgentsDoc, TypeSettings, TypeMCP, TypeRule, TypeHook:
	default:
		return UnsupportedAssetTypeError{Type: m.Type}
	}
	return nil
}

// Load reads and validates one asset directory.
func Load(dir string) (*Asset, errs.DomainError) {
	var manifest Manifest
	if err := utils.ReadJSON(filepath.Join(dir, config.AssetManifestFileName), &manifest); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &Asset{Manifest: manifest, Dir: dir}, nil
}

// Init scaffolds a new asset directory with a starter file layout that matches
// the chosen type. Returns the directory path if successful
func Init(root string, manifest Manifest) (string, errs.DomainError) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)
	if err := utils.EnsureDir(dir); err != nil {
		return "", err
	}
	if err := utils.WriteJSON(filepath.Join(dir, config.AssetManifestFileName), manifest); err != nil {
		return "", err
	}

	// Per-type starter content is currently inline; task 0010 tracks
	// extracting this into a strategy + template package.
	switch manifest.Type {
	case TypeSkill:
		body := []byte("---\nname: " + manifest.Name + "\ndescription: " + manifest.Description + "\n---\n\nDescribe the skill here.\n")
		if err := utils.WriteFile(filepath.Join(dir, config.SkillStarterFileName), body, 0o644); err != nil {
			return "", err
		}
	case TypeAgentsDoc:
		if err := utils.WriteFile(filepath.Join(dir, config.AgentsDocStarterFileName), []byte("# "+manifest.Name+"\n"), 0o644); err != nil {
			return "", err
		}
	case TypeSettings:
		if err := utils.WriteFile(filepath.Join(dir, config.SettingsStarterFileName), []byte("# codex settings\n"), 0o644); err != nil {
			return "", err
		}
	}
	// TypeMCP, TypeRule, TypeHook intentionally produce no starter file —
	// their content is user-authored and the directory is left ready for it.
	// Manifest.Validate above rejects unknown types, so no default branch is
	// needed.

	return dir, nil
}

// InitFromFolder creates a new asset whose content is copied from an
// existing folder instead of scaffolded from a starter template. It is
// the source-from-folder counterpart of Init: the manifest is validated,
// the asset directory is created, sourceDir's files are copied in, and the
// asset.json manifest is written last so a stray asset.json in the source
// cannot clobber the authoritative manifest. Returns the directory path.
func InitFromFolder(root string, manifest Manifest, sourceDir string) (string, errs.DomainError) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)
	if err := utils.EnsureDir(dir); err != nil {
		return "", err
	}
	if err := utils.CopyDir(sourceDir, dir); err != nil {
		return "", err
	}
	if err := utils.WriteJSON(filepath.Join(dir, config.AssetManifestFileName), manifest); err != nil {
		return "", err
	}
	return dir, nil
}

// Delete removes the asset directory at dir. A pre-missing directory is
// treated as success so the operation is idempotent — symmetric with
// project.Delete.
func Delete(dir string) errs.DomainError {
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return AssetFolderRemoveError{Dir: dir, Err: err}
	}
	return nil
}

// SaveManifest overwrites the asset manifest in dir with manifest. The
// caller is responsible for ensuring dir is the authoritative directory
// for this asset (typically resolved from a loaded profile).
func SaveManifest(dir string, manifest Manifest) errs.DomainError {
	if err := manifest.Validate(); err != nil {
		return err
	}
	return utils.WriteJSON(filepath.Join(dir, config.AssetManifestFileName), manifest)
}

// SortByName sorts list in place by display name ascending. It is the
// single source of truth for asset ordering used by both
// profile.AssetList and screens that derive their own subsets, so the
// rule does not drift between callers.
func SortByName(list []*Asset) {
	slices.SortFunc(list, func(a, b *Asset) int {
		return strings.Compare(a.Name, b.Name)
	})
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
func RelativeFiles(root string) ([]string, errs.DomainError) {
	var files []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := utils.ToRelative(root, path)
		if rel == config.AssetManifestFileName || strings.HasPrefix(rel, ".") {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if walkErr != nil {
		return nil, AssetWalkError{AssetDir: root, Err: walkErr}
	}
	return files, nil
}
