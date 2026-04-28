---
id: 0011
type: task
status: Pending
topics: go, render
---

# Replace render's `addAssetOutputs` switch with `(Type, Agent)` strategy lookup

`render.addAssetOutputs` and `render.addSkillOutputs`
(`internal/render/render.go`) hold all per-(asset-type, agent) render
rules in one big switch. New asset types or new agents require editing
both functions and risk drift.

Introduce a strategy interface keyed by `(asset.Type, agent string)` so
each combination has a tested unit. Lookup becomes
`strategy[Type+Agent].render(...)` and per-strategy tests stay focused.
The current logic for `skill` per agent (`codex`, `claude-code`,
`opencode` → `<root>/skills/<id>/...`; `cursor` → single
`.cursor/commands/<id>.md`) becomes four strategies; `agents_doc`,
`settings`, `mcp`/`rule`/`hook` (generic projections) get their own.

Originally tracked as a `// TODO:` block in `render.go`.
