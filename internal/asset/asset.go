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

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/surfaces"
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
	Agent  agent.Agent `json:"agent"`
	Source string      `json:"source"`
	Target string      `json:"target"`
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
	// Version is the asset.json schema version. Legacy manifests written
	// before versioning carry no key and decode to 0, the legacy sentinel
	// that Migrate stamps up to Version on load.
	Version     int    `json:"version"`
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
	// for. Empty means "every enabled agent" (see SupportsAgent). Each value
	// must be a recognized agent.Agent (codex, claude-code, cursor,
	// opencode); Manifest.Validate rejects unknown ids at load time so a typo
	// surfaces as an error instead of silently rendering nothing.
	CompatibleAgents []agent.Agent `json:"compatible_agents,omitempty"`
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

// Version is the current asset.json schema version.
const Version = 1

// Migrate stamps the legacy sentinel to Version; see utils.Persisted.
func (m *Manifest) Migrate() errs.DomainError {
	if m.Version == 0 {
		m.Version = Version
	}
	return nil
}

// SchemaVersion reports this manifest's version and the current one; see
// utils.Persisted.
func (m *Manifest) SchemaVersion() (have, known int) { return m.Version, Version }

// Validate checks only the domain-level shape of the manifest (id/name/type
// and agent references). It does not inspect agent-specific projection
// semantics; the version guard is owned by the boundary (see SchemaVersion).
func (m *Manifest) Validate() errs.DomainError {
	if m.ID == "" || m.Name == "" {
		return ErrAssetIDNameRequired
	}
	switch m.Type {
	case TypeSkill, TypeAgentsDoc, TypeSettings, TypeMCP, TypeRule, TypeHook:
	default:
		return UnsupportedAssetTypeError{Type: m.Type}
	}
	if unknown := unknownAgents(*m); len(unknown) > 0 {
		return UnknownCompatibleAgentError{Agents: unknown}
	}
	return nil
}

// unknownAgents collects every unrecognized agent id referenced by the
// manifest — both CompatibleAgents and per-projection agents — so a single
// UnknownCompatibleAgentError can report all of them at once. The two sources
// are concatenated and handed to agent.Unknown, which owns the order-preserving
// dedup rule (shared with project validation).
func unknownAgents(m Manifest) []agent.Agent {
	ids := append([]agent.Agent(nil), m.CompatibleAgents...)
	for _, p := range m.Projections {
		ids = append(ids, p.Agent)
	}
	return agent.Unknown(ids)
}

// Load reads one asset directory. ReadJSON migrates and validates the
// manifest at the persistence boundary, so no separate Validate call is
// needed here.
func Load(dir string) (*Asset, errs.DomainError) {
	manifest, err := utils.ReadJSON[Manifest](filepath.Join(dir, config.AssetManifestFileName))
	if err != nil {
		return nil, err
	}
	return &Asset{Manifest: manifest, Dir: dir}, nil
}

// Init scaffolds a new asset directory with a starter file layout that matches
// the chosen type. Returns the directory path if successful
func Init(root string, manifest Manifest) (string, errs.DomainError) {
	return scaffold(root, manifest, func(dir string) errs.DomainError {
		return writeStarter(dir, manifest)
	})
}

// InitFromFolder is the source-from-folder counterpart of Init: instead of a
// starter template, the asset's content is copied from sourceDir. The manifest
// is written last (see scaffold) so a stray asset.json in the source cannot
// clobber the authoritative one. Returns the directory path.
func InitFromFolder(root string, manifest Manifest, sourceDir string) (string, errs.DomainError) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	if err := validateFolderSource(manifest, sourceDir); err != nil {
		return "", err
	}
	return scaffold(root, manifest, func(dir string) errs.DomainError {
		return utils.CopyDir(sourceDir, dir)
	})
}

// scaffold owns the on-disk layout rule shared by Init and InitFromFolder:
// validate the manifest, derive the asset directory (assets/<type>/<id>/),
// create it, seed its content, and write asset.json last. Writing the
// manifest after seeding means a seed step that lands its own asset.json
// (e.g. a folder copy) cannot clobber the authoritative manifest.
func scaffold(root string, manifest Manifest, seed func(dir string) errs.DomainError) (string, errs.DomainError) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)
	if err := utils.EnsureDir(dir); err != nil {
		return "", err
	}
	if seed != nil {
		if err := seed(dir); err != nil {
			return "", err
		}
	}
	if err := utils.WriteJSON(filepath.Join(dir, config.AssetManifestFileName), manifest); err != nil {
		return "", err
	}
	return dir, nil
}

// writeStarter seeds the per-type starter content for a freshly scaffolded
// asset by looking up the type's entry in the starters table (see starter.go),
// rendering its embedded template, and writing the result. TypeMCP, TypeRule,
// and TypeHook have no entry and produce no starter file — their content is
// user-authored. Manifest.Validate rejects unknown types, so no default branch
// is needed.
func writeStarter(dir string, manifest Manifest) errs.DomainError {
	s, ok := starters[manifest.Type]
	if !ok {
		return nil
	}
	body, err := renderStarter(s, manifest)
	if err != nil {
		return err
	}
	return utils.WriteFile(filepath.Join(dir, s.filename), body, 0o644)
}

// RequiredContentFile returns the source file a folder must contain to be
// registerable as the given type, and whether the type has such a
// requirement. It derives from the same starters table as writeStarter, so the
// convention-based types (skill, agents_doc, settings) map to exactly the
// filename their starter writes; the generic types (mcp, rule, hook) have no
// entry — they render via explicit projections the folder flow does not
// collect.
func RequiredContentFile(t Type) (string, bool) {
	s, ok := starters[t]
	return s.filename, ok
}

// FolderRegisterableTypes returns the asset types whose content can come
// straight from a folder — exactly the types that declare a required content
// file. The generic types are excluded because the folder-register flow does
// not collect the projections they need to render.
func FolderRegisterableTypes() []Type {
	var out []Type
	for _, t := range AllTypes() {
		if _, ok := RequiredContentFile(t); ok {
			out = append(out, t)
		}
	}
	return out
}

// validateFolderSource checks that sourceDir satisfies the render contract for
// manifest.Type before the folder becomes a profile-owned asset, so a re-plan
// can actually reclassify the copied files as managed. Convention-based types
// must contain their required file; generic types must carry non-empty
// projections that all stay inside the managed-surface fence.
func validateFolderSource(manifest Manifest, sourceDir string) errs.DomainError {
	if required, ok := RequiredContentFile(manifest.Type); ok {
		if !utils.Exists(filepath.Join(sourceDir, required)) {
			return MissingContentFileError{Type: manifest.Type, File: required}
		}
		return nil
	}
	if len(manifest.Projections) == 0 {
		return MissingProjectionsError{Type: manifest.Type}
	}
	for _, p := range manifest.Projections {
		if !surfaces.IsAllowed(p.Target) {
			return ProjectionOutsideSurfacesError{Target: p.Target}
		}
	}
	return nil
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
	// WriteJSON migrates then validates before persisting, so an invalid
	// manifest is never written and no separate Validate call is needed.
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
func SupportsAgent(a *Asset, ag agent.Agent) bool {
	if len(a.CompatibleAgents) == 0 {
		return true
	}
	return slices.Contains(a.CompatibleAgents, ag)
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
