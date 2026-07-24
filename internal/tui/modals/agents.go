// Package modals contains the typed `huh.Form`-backed modal constructors used
// by the TUI screens. Each modal exposes a `New(...)` constructor and a typed
// result struct so `*huh.Form` does not leak out of this package.
package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/config"
)

// Agent identifier re-exports as plain strings. The canonical typed source
// lives in internal/config; the modals package binds huh form state as
// `[]string` (huh multiselect binds strings), so these string re-exports let
// call sites here (option lists, tests) stay in the string world while the
// renderer and project model share the typed config.Agent.
const (
	AgentCodex      = string(config.AgentCodex)
	AgentClaudeCode = string(config.AgentClaudeCode)
	AgentCursor     = string(config.AgentCursor)
	AgentOpenCode   = string(config.AgentOpenCode)
)

// AgentOptions returns the closed set of agent options for `huh.MultiSelect`.
// The list is built from `config.AllAgents()` so adding a new agent id only
// needs an edit to `internal/config/agents.go`.
func AgentOptions() []huh.Option[string] {
	agents := config.AllAgents()
	out := make([]huh.Option[string], 0, len(agents))
	for _, a := range agents {
		out = append(out, huh.NewOption(a.String(), a.String()))
	}
	return out
}
