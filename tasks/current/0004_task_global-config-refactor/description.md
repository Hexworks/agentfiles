---
id: 0004
type: task
status: in-review
topics: go, research
---

# Move all configuration into a single module.

Currently we have configuration scattered around the application like:

```go
for _, root := range []string{"AGENTS.md", ".claude", ".cursor", ".codex", ".opencode", ".mcp.json"} {
```

Here we inlined the files that we're looking for. We should have global config variables instead that we can change in one place:

```go
package config

var SomethingConfig = []string{
	"AGENTS.md",
	".claude/",
	".cursor/",
	".codex/",
	".opencode/",
	".mcp.json",
}
```

## Plan

[plan.md](./plan.md)
