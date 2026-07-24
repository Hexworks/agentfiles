package render

import (
	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/surfaces"
)

// strategyKey is the typed (agent, asset-type) pair the render pipeline
// dispatches on. Both fields are enum-like `type X string` with fixed
// constant sets, so the table can never be keyed by a bare string.
type strategyKey struct {
	Agent agent.Agent
	Type  asset.Type
}

// ReverseContext carries everything a Strategy needs to map a managed
// repo path back to its asset-relative source path for Adopt (ADR 0020).
//
// RepoPath is the forward-slash, repo-relative path being reversed.
// When RepoPath exactly matches a rendered file, ExactSourceRel holds
// that file's recorded SourceRel — the authoritative answer that already
// captures any filename change (agents_doc AGENTS.md→CLAUDE.md, settings
// codex.toml→config.toml, a single-file generic projection rename).
// TargetRoot / SourceRoot describe the owning rendered directory and are
// used only for an *unknown* sibling file (ExactSourceRel empty), where
// the source path is derived by preserving the tail below TargetRoot.
type ReverseContext struct {
	RepoPath       string
	TargetRoot     string
	SourceRoot     string
	AssetID        string
	ExactSourceRel string
}

// Strategy is one self-contained (agent, type) rendering unit. It owns
// both directions so forward render and reverse Adopt cannot drift:
//
//   - Render emits the files this pair produces for one asset. It is
//     pure — it reads asset source files and returns them in memory,
//     never writing the target repo (Invariant #1: plan before apply).
//   - Reverse maps a managed repo path back to the asset-relative source
//     path. ok is false when the mapping is lossy — the strategy is the
//     authority on its own invertibility (cursor's flat-file skill layout
//     drops the filename and reports ok=false).
type Strategy interface {
	Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError)
	Reverse(ctx ReverseContext) (sourceRel string, ok bool)
}

// registry is the global lookup table of strategies, built once at
// package load. Dispatch is always a map lookup, never a switch, so
// adding an agent or asset type is a table edit rather than a change to
// branching control flow.
var registry = buildRegistry()

// buildRegistry assembles every recognized (agent, type) pair. Pairs
// that share identical logic (the three skill-folder agents, the twelve
// generic-projection pairs) reuse a parameterized struct but are still
// registered explicitly under their own key. A pair absent from the
// table is a deliberate omission (cursor has no skill-folder entry) and
// surfaces as UnsupportedRenderingError when a project selects it.
func buildRegistry() map[strategyKey]Strategy {
	reg := map[strategyKey]Strategy{}

	// skill: folder-per-skill for the container-root agents, flat file
	// for cursor.
	for _, ag := range agent.All() {
		if root, ok := surfaces.SkillRoot(ag.String()); ok {
			reg[strategyKey{ag, asset.TypeSkill}] = skillFolderStrategy{root: root}
		}
	}
	reg[strategyKey{agent.Cursor, asset.TypeSkill}] = cursorSkillStrategy{}

	// agents_doc: source is always AGENTS.md; only the target filename
	// differs per agent (CLAUDE.md for claude-code, AGENTS.md otherwise).
	for _, ag := range agent.All() {
		target := config.AgentsDocStarterFileName
		if ag == agent.ClaudeCode {
			target = config.ClaudeDocFileName
		}
		reg[strategyKey{ag, asset.TypeAgentsDoc}] = agentsDocStrategy{target: target}
	}

	// settings: one per agent, driven by the descriptor table the agent
	// package owns.
	for _, d := range agent.Descriptors() {
		reg[strategyKey{d.Agent, asset.TypeSettings}] = settingsStrategy{descriptor: d}
	}

	// generic projection types: every agent shares one projection-walk
	// strategy filtered by the projection's own agent.
	for _, ag := range agent.All() {
		for _, t := range []asset.Type{asset.TypeMCP, asset.TypeRule, asset.TypeHook} {
			reg[strategyKey{ag, t}] = genericProjectionStrategy{}
		}
	}

	return reg
}

// strategyFor returns the strategy registered for the (agent, type)
// pair. ok is false for an unregistered pair; callers turn that into an
// accumulated UnsupportedRenderingError (Build) or a failed reverse
// lookup (ReverseLookup).
func strategyFor(ag agent.Agent, t asset.Type) (Strategy, bool) {
	s, ok := registry[strategyKey{ag, t}]
	return s, ok
}
