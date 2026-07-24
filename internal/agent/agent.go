// Package agent owns the closed set of AI-assistant identifiers the renderer
// targets. It is the single source of truth for which agents exist, how they
// are validated, and the per-agent render conventions keyed off them.
//
// The identifier is a named string type, so on-disk JSON (enabled_agents,
// compatible_agents) round-trips identically to the previous untyped form — no
// migration. The package is a leaf: it imports only the standard library, so
// every domain package (asset, project, render, app, tui) can import it without
// introducing a cycle.
package agent

// Agent is the typed identifier for an AI assistant. project.Manifest keys its
// EnabledAgents off it, asset.Manifest restricts CompatibleAgents to it, and
// render selects per-agent output conventions by it. Validation (IsKnown /
// Unknown) rejects values outside the recognized set at manifest load time so a
// typo surfaces as an error instead of silently rendering nothing.
type Agent string

// The recognized agent identifiers. Adding a new agent requires a constant
// here plus an entry in the knownAgents set, the All slice, and (if it renders
// settings) Descriptors.
const (
	Codex      Agent = "codex"
	ClaudeCode Agent = "claude-code"
	Cursor     Agent = "cursor"
	OpenCode   Agent = "opencode"
)

// String returns the underlying identifier so call sites that still speak the
// string-keyed vocabulary (surfaces path registry, huh form options) can bridge
// without a manual conversion.
func (a Agent) String() string { return string(a) }

// knownAgents backs IsKnown with an O(1) membership test built once at package
// load, so validating a manifest with N agents does not allocate N throwaway
// slices.
var knownAgents = map[Agent]struct{}{
	Codex:      {},
	ClaudeCode: {},
	Cursor:     {},
	OpenCode:   {},
}

// All returns every recognized agent in stable order. It allocates a fresh
// slice so callers may iterate or sort it without mutating package state.
func All() []Agent {
	return []Agent{Codex, ClaudeCode, Cursor, OpenCode}
}

// IsKnown reports whether a is one of the recognized agents.
func IsKnown(a Agent) bool {
	_, ok := knownAgents[a]
	return ok
}

// Unknown returns the unrecognized agents in as, preserving first-seen order
// and deduplicating, so a caller can report every bad id in one pass. It is the
// single owner of the "collect unknown agents" rule shared by asset and project
// validation. Nil-safe (nil in, nil out).
func Unknown(as []Agent) []Agent {
	var unknown []Agent
	seen := map[Agent]struct{}{}
	for _, a := range as {
		if IsKnown(a) {
			continue
		}
		if _, dup := seen[a]; dup {
			continue
		}
		seen[a] = struct{}{}
		unknown = append(unknown, a)
	}
	return unknown
}

// FromStrings converts a string slice into typed agents. Nil-safe (nil in, nil
// out) and defensively copied so callers can keep the source slice. Used at the
// TUI form boundary where huh binds []string.
func FromStrings(ss []string) []Agent {
	if ss == nil {
		return nil
	}
	out := make([]Agent, len(ss))
	for i, s := range ss {
		out[i] = Agent(s)
	}
	return out
}

// Strings is the inverse of FromStrings: typed agents to the []string the huh
// form world binds. Nil-safe and defensively copied.
func Strings(as []Agent) []string {
	if as == nil {
		return nil
	}
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = string(a)
	}
	return out
}

// Descriptor bundles the per-agent render conventions previously inlined in
// internal/render: the settings source filename authored in the profile and the
// well-known settings target path written into the repo. Centralising the table
// here keeps the recognized set and its render conventions in one owner. Skill
// container roots stay in internal/surfaces (the path registry); consolidating
// the remaining per-agent render conventions is deferred to task 0011.
type Descriptor struct {
	Agent          Agent
	SettingsSource string
	SettingsTarget string
}

// Descriptors returns the per-agent settings render conventions in stable
// order. render iterates it to project each enabled agent's settings file.
func Descriptors() []Descriptor {
	return []Descriptor{
		{Codex, "codex.toml", ".codex/config.toml"},
		{ClaudeCode, "claude-code.json", ".claude/settings.local.json"},
		{Cursor, "cursor.json", ".cursor/config.json"},
		{OpenCode, "opencode.json", ".opencode/config.json"},
	}
}
