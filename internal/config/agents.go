package config

import "slices"

// Agent is the typed identifier for an AI assistant the renderer targets: the
// renderer keys per-agent projections off it, project.Manifest.EnabledAgents
// stores them, asset.Manifest.CompatibleAgents restricts to them, and the TUI
// presents them as a closed set in agent-picker forms. Centralised here so
// render, project, asset, and the TUI modals all reference one source of
// truth. It is a named string type, so on-disk JSON round-trips identically to
// the previous untyped form (no migration).
type Agent string

// The recognized agent identifiers. Adding a new agent requires a constant
// here and an entry in AllAgents below; IsKnownAgent then rejects anything
// outside that set at manifest load time.
const (
	AgentCodex      Agent = "codex"
	AgentClaudeCode Agent = "claude-code"
	AgentCursor     Agent = "cursor"
	AgentOpenCode   Agent = "opencode"
)

// String returns the underlying identifier so call sites that still speak the
// string-keyed vocabulary (surfaces path registry, huh form options) can
// bridge without a manual conversion.
func (a Agent) String() string { return string(a) }

// AllAgents returns every supported agent identifier in stable order.
// Adding a new agent requires updating this slice and the constants above;
// callers iterating it then pick the change up automatically.
func AllAgents() []Agent {
	return []Agent{
		AgentCodex,
		AgentClaudeCode,
		AgentCursor,
		AgentOpenCode,
	}
}

// IsKnownAgent reports whether a is one of the recognized agent identifiers.
// The manifest Validate methods in asset and project use it to reject unknown
// values at load time.
func IsKnownAgent(a Agent) bool {
	return slices.Contains(AllAgents(), a)
}

// ToAgents converts a string slice into typed agents. It is nil-safe (nil in,
// nil out) and copies defensively so callers can keep the source slice. Used
// at the TUI form boundary where huh binds `[]string`.
func ToAgents(ss []string) []Agent {
	if ss == nil {
		return nil
	}
	out := make([]Agent, len(ss))
	for i, s := range ss {
		out[i] = Agent(s)
	}
	return out
}

// AgentStrings is the inverse of ToAgents: typed agents to the `[]string` the
// huh form world binds. Nil-safe and defensively copied.
func AgentStrings(as []Agent) []string {
	if as == nil {
		return nil
	}
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = string(a)
	}
	return out
}
