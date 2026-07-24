---
id: 0011
type: task
status: pending
topics: go, render
---

# Replace render's `(Type, Agent)` switch with an isomorphic strategy lookup

`render.addRenderedFilesFor` and `render.addSkillOutputs`
(`internal/render/render.go`) hold all per-(asset-type, agent) render
rules in one big switch. New asset types or new agents require editing
both functions and risk drift.

> [!NOTE]
> The task title originally named `addAssetOutputs`; the function was
> since renamed to `addRenderedFilesFor`. Same switch, same TODO.

Introduce a strategy interface keyed by `(asset.Type, agent.Agent)` so
each combination has a tested unit. Forward lookup becomes
`strategy[Type, Agent].Render(...)` and per-strategy tests stay focused.
The current logic for `skill` per agent (`codex`, `claude-code`,
`opencode` → `<root>/skills/<id>/...`; `cursor` → single
`.cursor/commands/<id>.md`) becomes distinct strategies; `agents_doc`,
`settings`, and the generic `mcp`/`rule`/`hook` projections get their own.

Originally tracked as a `// TODO:` block in `render.go`.

**Update — isomorphism (Adopt).** The strategy must own **both**
directions because of "Adopt" (ADR 0020). Forward, rendering a skill to
claude does:

`assets/skill/af.check-task/SKILL.md` → `.claude/skills/af.check-task/SKILL.md`

Reverse, adopting a claude skill must do:

`.claude/skills/af.check-task/SKILL.md` → `assets/skill/af.check-task/SKILL.md`

Today the **reverse** mapping lives in `internal/sync`
(`assetProjectionDirs` + `owningAssetSourceRelFor`), reconstructed from
each `render.RenderedFile.SourceRel` by suffix-stripping. This task moves
that reverse logic **into the strategy** so forward and reverse cannot
drift: each strategy exposes `Render` and `Reverse`, and `sync` delegates
to the strategy instead of re-deriving the mapping. A strategy can affect
file **paths** (skill/settings/agents_doc examples above) or file
**content** (agent-specific config transforms), so both must round-trip.

## Clarification

### Question

The reverse mapping already exists in `internal/sync`. How far does the
`(Type, Agent)` strategy go on the "isomorphic" requirement — forward-only
with a round-trip test, or own both directions and have sync delegate?

### Answer

**Own both directions.** The strategy exposes `Render` (forward) and
`Reverse` (repo path → `(assetID, sourceRel)`). `sync`'s
`assetProjectionDirs` / `owningAssetSourceRelFor` reverse logic is
replaced by delegation to the strategy table, so the two directions live
in one tested unit and cannot drift. Larger blast radius (touches `sync`)
is accepted for the drift guarantee.

### Question

Cursor's skill layout flattens a multi-file skill into a single
`.cursor/commands/<id>.md`, dropping the source filename. Is that
invertible?

### Answer

No — it is intentionally lossy and stays that way. Cursor skill files are
**not adoptable** today (`.cursor/commands` hosts files from many assets,
so sync classifies the dir as ambiguous and drops it). The Cursor skill
strategy therefore renders forward but its `Reverse` reports
"not invertible / no owner", preserving current behavior. No new Adopt
capability is introduced for Cursor skills.

## Acceptance Criteria

Boundary-crossing criteria exercise the **real** render + sync stack
against the real filesystem in `t.TempDir()` and assert bytes/paths on
disk, not a constant the code also produced. Behavior must stay
observably identical to today for every `(Type, Agent)` pair.

- [ ] `make fmt && make test && make lint && make build` all pass.
- [ ] `render.Build` output is byte- and path-identical to the pre-refactor
      switch for every `(Type, Agent)` combination — a golden test
      (`TestBuild_AllTypeAgentPairs_Unchanged`) renders a fixture profile
      with every asset type across all four agents and asserts the full
      `[]RenderedFile` (Path, Body, Mode, AssetID, SourceRel).
- [ ] `TestStrategy_SkillFolderAgents_RoundTrip`: for each of `codex`,
      `claude-code`, `opencode`, a multi-file skill rendered into
      `t.TempDir()` and then reversed via the strategy recovers the exact
      `(assetID, sourceRel)` for every file (forward∘reverse == identity).
- [ ] `TestStrategy_CursorSkill_NotInvertible`: the Cursor skill strategy
      renders `.cursor/commands/<id>.md` forward but `Reverse` reports no
      owner; a full real `sync.Plan`/`Apply` over a repo containing that
      file offers **no** Adopt row for it (matches today).
- [ ] `TestStrategy_Settings_RoundTrip`: settings for each agent render to
      the `agent.Descriptors()` target and reverse back to the descriptor
      source (`.codex/config.toml` ⇄ `codex.toml`, etc.).
- [ ] `TestStrategy_AgentsDoc_RoundTrip`: `agents_doc` renders to
      `AGENTS.md` for Codex only and reverses to `AGENTS.md` source.
- [ ] `TestStrategy_GenericProjection_RoundTrip`: `mcp`/`rule`/`hook`
      explicit projections render to target and reverse to source,
      including a directory (walked) projection.
- [ ] Adopt end-to-end unchanged: real `sync.Apply` over a drifted managed
      skill file returns an `AdoptRequest` whose `(AssetID, SourceRel)`
      equals the strategy's `Reverse` result, and `app.Service.Apply`
      writes the local body back to `assets/skill/<id>/<rel>` on disk —
      asserted by reading the profile asset file after Apply.
- [ ] Single source of truth: `grep -n "switch" internal/render/render.go`
      shows no `(Type, Agent)` switch remains, and `sync`'s
      `assetProjectionDirs` / `owningAssetSourceRelFor` no longer
      re-derive the mapping (they delegate to the strategy).

## Out of scope

- Adding any new asset type or new agent.
- Changing any on-disk layout, target path, or file content for any
  existing `(Type, Agent)` pair — this is a pure refactor; observable
  output must not change.
- Making Cursor skills adoptable.
- Moving `asset.Type` or `agent.Agent` to a different package.

## Verification

1. `make fmt && make test && make lint && make build` — all green.
2. `go test ./internal/render/... ./internal/sync/...` — new strategy and
   round-trip tests pass alongside the pre-existing render/sync suites.
3. `grep -n "switch" internal/render/render.go` — no `(Type, Agent)` or
   `a.Type` dispatch switch remains in `addRenderedFilesFor` /
   `addSkillOutputs` (both replaced by a strategy-table lookup).
4. `grep -n "assetProjectionDirs\|owningAssetSourceRelFor" internal/sync/sync.go`
   — reverse mapping delegates to the render strategy rather than
   suffix-stripping `SourceRel` inline.
