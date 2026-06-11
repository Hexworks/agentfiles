// Package modals contains the typed `huh.Form`-backed modal constructors used
// by the TUI screens. Each modal exposes a `New(...)` constructor and a typed
// result struct so `*huh.Form` does not leak out of this package.
package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/config"
)

// Agent identifier re-exports. The canonical source lives in
// internal/config; the modals package re-binds them so call sites here
// (option lists, tests) read as TUI-local while the renderer and project
// model share the same constants.
const (
	AgentCodex      = config.AgentCodex
	AgentClaudeCode = config.AgentClaudeCode
	AgentCursor     = config.AgentCursor
	AgentOpenCode   = config.AgentOpenCode
)

// AgentOptions returns the closed set of agent options for `huh.MultiSelect`.
// The list is built from `config.AllAgents()` so adding a new agent id only
// needs an edit to `internal/config/agents.go`.
func AgentOptions() []huh.Option[string] {
	agents := config.AllAgents()
	out := make([]huh.Option[string], 0, len(agents))
	for _, a := range agents {
		out = append(out, huh.NewOption(a, a))
	}
	return out
}
