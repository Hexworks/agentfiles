package config

// Agent identifiers used across the codebase: the renderer keys per-agent
// projections off them, project.Manifest.EnabledAgents stores them, and the
// TUI presents them as a closed set in agent-picker forms. Centralised here
// so render, project, and the TUI modals all reference one source of truth.
const (
	AgentCodex      = "codex"
	AgentClaudeCode = "claude-code"
	AgentCursor     = "cursor"
	AgentOpenCode   = "opencode"
)

// AllAgents returns every supported agent identifier in stable order.
// Adding a new agent requires updating this slice and the constants above;
// callers iterating it then pick the change up automatically.
func AllAgents() []string {
	return []string{
		AgentCodex,
		AgentClaudeCode,
		AgentCursor,
		AgentOpenCode,
	}
}
