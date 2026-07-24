---
id: 0009
type: task
status: in-review
topics: go, asset
---

# Replace `Manifest.CompatibleAgents []string` with a typed value

`asset.Manifest.CompatibleAgents` is currently `[]string`
(`internal/asset/asset.go`). Any string passes JSON parsing; only
`asset.SupportsAgent` checks membership at render time. A typo in a
manifest renders silently as "no compatible agents" without a clear
error.

Replace the field with a typed `CompatibleAgent` value (or enum-like
constant set) that mirrors the recognized agents list (`codex`,
`claude-code`, `cursor`, `opencode`). Validation should reject unknown
values at load time with a typed error.

Originally tracked as a `// FIX:` marker in `asset.go`.

## Clarification

### Question

How wide should the typed-agent change go — narrow (add `config.Agent`
alongside the existing untyped consts; only `asset.CompatibleAgents` typed) or
broad (retype the agent identifier everywhere: config consts, `EnabledAgents`,
`Projection.Agent`, render, project, actions, TUI)?

### Answer

Broad — retype everything. One canonical `config.Agent` type across the
domain; huh form-state fields stay `[]string` and convert at the modal
boundary.
