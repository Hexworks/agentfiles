---
id: 0011
type: task
status: in-review
topics: go, render
---

# Replace render's `(Type, Agent)` switch with a per-(Agent,Type) strategy table

`render.addRenderedFilesFor` and `render.addSkillOutputs`
(`internal/render/render.go`) hold all per-(asset-type, agent) render
rules in one big switch. New asset types or new agents require editing
both functions and risk drift.

> [!NOTE]
> The task title originally named `addAssetOutputs`; the function was
> since renamed to `addRenderedFilesFor`. Same switch, same TODO.

Introduce a **global lookup table** of strategies, one per
`(agent.Agent, asset.Type)` pair — e.g. `(ClaudeCode, Skill)`. Each
strategy is a small self-contained implementation that knows only two
operations: `Render` (forward) and `Reverse` (back, for Adopt). Dispatch
becomes a **map lookup, never a switch**. There is no name-special-cased
`cursorSkillStrategy`: `(Cursor, Skill)` is just another table entry.

Originally tracked as a `// TODO:` block in `render.go`.

**Isomorphism (Adopt).** The strategy owns **both** directions because of
"Adopt" (ADR 0020). Forward, rendering a skill to claude does:

`assets/skill/af.check-task/SKILL.md` → `.claude/skills/af.check-task/SKILL.md`

Reverse, adopting a claude skill must do:

`.claude/skills/af.check-task/SKILL.md` → `assets/skill/af.check-task/SKILL.md`

Today the reverse mapping lives in `internal/sync`
(`assetProjectionDirs` + `owningAssetSourceRelFor`). This task moves it
into the strategy so forward/reverse cannot drift: `sync` delegates to
`render.ProjectPlan.ReverseLookup`. A strategy can transform file
**paths** (skill/settings/agents_doc) or file **content** (agent config),
so both must round-trip. A strategy is the authority on its own
invertibility — a lossy one (cursor skill flat-file) reports `ok=false`.

## Clarification

### Question

How far does the strategy go on the "isomorphic" requirement — forward
only, or own both directions with sync delegating?

### Answer

**Own both directions.** `Render` (forward) and `Reverse`
(repo path → `(assetID, sourceRel)`). `sync`'s reverse re-derivation is
deleted; `sync` calls `render.ProjectPlan.ReverseLookup`. Larger blast
radius (touches `sync`) accepted for the drift guarantee.

### Question

What is the strategy keyed by, and how are missing entries handled?

### Answer

Keyed by a typed `(agent.Agent, asset.Type)` pair (both are enum-like
`type X string` with fixed constants — no bare strings). Strategies live
in one global lookup table. A lookup miss returns a
`UnsupportedRenderingError{Agent, Type}` DomainError (e.g. "missing
render strategy for cursor skill"), **accumulated** with any other render
errors via the existing `errs`-slice / `errors.Join` pattern — never a
short-circuit.

### Question

`agents_doc` today renders only for Codex → `AGENTS.md`. Should it be
per-agent?

### Answer

Yes — **in scope for this task**. `(ClaudeCode, AgentsDoc)` → `CLAUDE.md`;
`(Codex|Cursor|OpenCode, AgentsDoc)` → `AGENTS.md`. This is new behavior
(claude/cursor/opencode agents_doc render nothing today) and requires
adding `"CLAUDE.md"` to the managed-surface fence (`surfaces.go`). Parity
holds only for unchanged pairs; the new agents_doc pairs get their own
assertions.

### Question

Generic types `mcp`/`rule`/`hook` are projection-driven (arbitrary
per-agent targets in the manifest). How do they fit the table?

### Answer

Register `(agent, mcp/rule/hook)` strategies for all agents, each
delegating to one shared projection-walk helper filtered by the
projection's agent. Uniform table — no false "unsupported" errors for
valid combos.

### Question

The plan/preview view needs a "dry run" (show paths + contents without
writing to disk). New mechanism?

### Answer

No new mechanism. `render` is already pure: it reads source asset files
and returns `RenderedFile{Path, Body}` **in memory**, writing nothing.
The sole writer is `sync.Apply`/`writeRendered`. So dry-run = call
`render.Build` (or `sync.Plan`) and don't call `sync.Apply`, exactly as
today. Strategies must stay pure (`Render`/`Reverse` never write); this
preserves Invariant #1 (plan before apply).

### Question

Cursor's skill layout flattens a multi-file skill into a single
`.cursor/commands/<id>.md`, dropping the filename. Invertible?

### Answer

No — intentionally lossy, stays that way. `(Cursor, Skill).Reverse`
returns `ok=false`; cursor skills remain non-adoptable (matches today,
where `.cursor/commands` is treated as an ambiguous multi-asset dir).

## Acceptance Criteria

Boundary-crossing criteria exercise the **real** render + sync stack in
`t.TempDir()` and assert bytes/paths on disk, not a constant the code
also produced.

- [ ] `make fmt && make test && make lint && make build` all pass.
- [ ] `TestBuild_UnchangedPairs_Golden`: fixture profile rendered across
      all agents is byte-/path-identical to a golden captured from the
      **pre-refactor** `Build` for every unchanged pair (all skill, all
      settings, all generic, and `(Codex, AgentsDoc)`).
- [ ] `TestBuild_AgentsDoc_PerAgentTargets`: an `agents_doc` asset enabled
      for all four agents renders `CLAUDE.md` (claude-code) and `AGENTS.md`
      (codex, cursor, opencode) — asserted on `t.TempDir()` paths+bodies.
- [ ] `TestSurfaces_AllowsClaudeMd`: `surfaces.IsAllowed("CLAUDE.md")` is
      true and a real `render.Build`→`sync.Plan` over a claude-code
      project surfaces the `CLAUDE.md` change (not a fence rejection).
- [ ] `TestStrategyFor_MissingPair_AggregatesUnsupportedError`: an
      unregistered `(Agent, Type)` yields `ok=false`; `Build` accumulates a
      single joined `UnsupportedRenderingError` whose `Error()` names the
      agent and type.
- [ ] `TestReverse_SkillFolderAgents_RoundTrip`: for codex/claude-code/
      opencode, a multi-file skill rendered into `t.TempDir()` then
      `ReverseLookup`'d recovers `(assetID, sourceRel)` for every file,
      incl. an untracked sibling file in the skill dir.
- [ ] `TestReverse_CursorSkill_NotInvertible`: `(Cursor, Skill).Reverse`
      returns `ok=false`; a real `sync.Plan`/`Apply` offers **no** Adopt
      row for the flat file.
- [ ] `TestReverse_Settings_And_AgentsDoc_RoundTrip`: settings reverse to
      the descriptor source per agent; `CLAUDE.md` and `AGENTS.md` reverse
      to the agents_doc source.
- [ ] `TestReverse_GenericProjection_RoundTrip`: mcp/rule/hook projections
      (incl. a walked directory) reverse to source via `ReverseLookup`.
- [ ] `TestAdopt_SkillDrift_EndToEnd`: real `sync.Apply` over a drifted
      managed skill file returns an `AdoptRequest` whose `(AssetID,
      SourceRel)` equals `ReverseLookup`, and `app.Service.Apply` writes
      the local body back to `assets/skill/<id>/<rel>` — asserted by
      reading the profile asset off disk after Apply.
- [ ] `grep -n "switch" internal/render/render.go` shows no `a.Type`
      dispatch switch; `assetProjectionDirs`/`owningAssetSourceRelFor` are
      deleted from `internal/sync/sync.go`.
- [ ] **New ADR** `docs/adr/0021-*.md` records the render/reverse strategy
      model (rendering is a core part of the app and must be documented).
- [ ] **Diagrams** (mermaid): component diagram of render in
      `docs/architecture/05-building-block-view.md`; forward-render and
      reverse-Adopt sequence diagrams in
      `docs/architecture/06-runtime-view.md`.

## Out of scope

- Adding any new asset **type** or new **agent**.
- Changing on-disk layout/content for pairs other than the new per-agent
  `agents_doc` (everything else is a pure refactor).
- Making cursor skills adoptable.
- Moving `asset.Type` or `agent.Agent` to a different package.

## Verification

1. `make fmt && make test && make lint && make build` — all green.
2. `go test ./internal/render/... ./internal/sync/... ./internal/surfaces/...`
   — new strategy/reverse/agents_doc/fence tests pass alongside existing.
3. `grep -n "switch" internal/render/render.go` — no `a.Type` dispatch.
4. `grep -n "assetProjectionDirs\|owningAssetSourceRelFor" internal/sync/sync.go`
   — gone; reverse delegates to `render.ProjectPlan.ReverseLookup`.
