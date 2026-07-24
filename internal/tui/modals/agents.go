// Package modals contains the typed `huh.Form`-backed modal constructors used
// by the TUI screens. Each modal exposes a `New(...)` constructor and a typed
// result struct so `*huh.Form` does not leak out of this package.
package modals

import (
	"charm.land/huh/v2"
	"github.com/hexworks/agentfiles/internal/agent"
)

// Agent identifier re-exports as plain strings. The canonical typed source
// lives in internal/agent; the modals package binds huh form state as
// `[]string` (huh multiselect binds strings), so these string re-exports let
// call sites here (option lists, tests) stay in the string world while the
// renderer and project model share the typed agent.Agent.
const (
	AgentCodex      = string(agent.Codex)
	AgentClaudeCode = string(agent.ClaudeCode)
	AgentCursor     = string(agent.Cursor)
	AgentOpenCode   = string(agent.OpenCode)
)

// AgentOptions returns the closed set of agent options for `huh.MultiSelect`.
// The list is built from `agent.All()` so adding a new agent id only needs an
// edit to `internal/agent/agent.go`.
func AgentOptions() []huh.Option[string] {
	agents := agent.All()
	out := make([]huh.Option[string], 0, len(agents))
	for _, a := range agents {
		s := a.String()
		out = append(out, huh.NewOption(s, s))
	}
	return out
}
