// Package modals contains the typed `huh.Form`-backed modal constructors used
// by the TUI screens. Each modal exposes a `New(...)` constructor and a typed
// result struct so `*huh.Form` does not leak out of this package.
package modals

import "charm.land/huh/v2"

// Agent identifiers recognized by the renderer. Kept in sync with
// project.Manifest.EnabledAgents.
const (
	AgentCodex       = "codex"
	AgentClaudeCode  = "claude-code"
	AgentCursor      = "cursor"
	AgentOpenCode    = "opencode"
)

// AgentOptions returns the closed set of agent options for `huh.MultiSelect`.
// The list is the single source of truth for the modals that ask the user to
// pick agents (Create Asset, Edit Project, Register Project).
func AgentOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("codex", AgentCodex),
		huh.NewOption("claude-code", AgentClaudeCode),
		huh.NewOption("cursor", AgentCursor),
		huh.NewOption("opencode", AgentOpenCode),
	}
}
