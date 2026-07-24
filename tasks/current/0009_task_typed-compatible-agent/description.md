---
id: 0009
type: task
status: pending
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
