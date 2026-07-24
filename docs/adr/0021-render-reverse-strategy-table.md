# Per-(Agent, Type) Render/Reverse Strategy Table

## Status

accepted

Refines [ADR 0020](./0020-adopt-as-sanctioned-reverse-flow.md) (Adopt is
the single sanctioned repo → profile flow): the reverse-mapping mechanics
ADR 0020 located in `internal/sync` move into `render`, so ADR 0021
supersedes ADR 0020 on **where** the reverse mapping lives (not on the
Adopt policy itself).

## Context

Rendering is the core of `agentfiles`: it turns a profile plus a project
manifest into the concrete files a repository should hold. The per-type
render rules used to live in one `switch a.Type` in
`render.addRenderedFilesFor` plus per-agent branching in
`render.addSkillOutputs`. Every new asset type or agent meant editing both
functions, and a forgotten arm rendered nothing silently.

Adopt (ADR 0020) needs the **reverse** of rendering: given a managed repo
path, find the asset source it was projected from. That reverse mapping
was re-derived independently inside `internal/sync`
(`assetProjectionDirs` + `owningAssetSourceRelFor`). Two independent
implementations of the same forward/reverse relationship can drift — a
forward render change that the reverse walk does not mirror silently
breaks Adopt.

Three forces shaped the decision:

- **No silent gaps.** A missing `(agent, type)` combination must be a
  typed, accumulated error, not a fall-through.
- **One owner per direction pair.** Forward render and reverse Adopt for a
  given `(agent, type)` must be authored together so they cannot diverge.
- **Purity.** Render must stay read-only (Invariant #1: plan before
  apply); only `sync.Apply` writes.

## Decision

Replace the type switch with a **global lookup table of strategies** keyed
by a typed `(agent.Agent, asset.Type)` pair. Both key fields are enum-like
`type X string` with fixed constant sets, so the table is never keyed by a
bare string. Dispatch is a **map lookup, never a switch**.

Each strategy is a small, self-contained unit that owns **both**
directions:

```go
type Strategy interface {
    Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError)
    Reverse(ctx ReverseContext) (sourceRel string, ok bool)
}
```

- `Render` (forward) emits the files this pair produces for one asset. It
  is pure — it reads asset sources and returns them in memory, never
  writing the repo.
- `Reverse` (back) maps a managed repo path to its asset-relative source
  for Adopt. The strategy is the **authority on its own invertibility**:
  a lossy layout reports `ok=false`.

Consequences of the model:

- **Missing pair → accumulated error.** `strategyFor(agent, type)` returns
  `ok=false` for an unregistered pair; `Build` appends an
  `UnsupportedRenderingError{Agent, Type}` (`"missing render strategy for
  <agent> <type>"`) and keeps going, never short-circuiting.
- **Reverse owned by render.** `render.ProjectPlan.ReverseLookup(repoPath)`
  is the single reverse entry point. `sync`'s `assetProjectionDirs` /
  `owningAssetSourceRelFor` are deleted; both call sites delegate to
  `ReverseLookup`. Forward and reverse now share one owner and cannot
  drift. `RenderedFile` gains `Agent` and `Type` so the reverse lookup can
  select the producing strategy.
- **Cursor stays non-adoptable.** `(Cursor, Skill)` flattens a whole skill
  into one `.cursor/commands/<id>.md`, dropping filenames; its `Reverse`
  returns `ok=false`, so a cursor command is never offered for Adopt — the
  same outcome the ambiguous-dir walk produced before.
- **Per-agent `agents_doc`.** The table registers `(ClaudeCode, AgentsDoc)`
  → `CLAUDE.md` and `(Codex|Cursor|OpenCode, AgentsDoc)` → `AGENTS.md`.
  This is new behavior — before, `agents_doc` rendered only for Codex. The
  managed-surface fence widens to include `CLAUDE.md` so the new target can
  be projected and synced. The asset's source file stays `AGENTS.md`
  regardless of agent; only the projected target differs.
- **Uniform generic types.** `(agent, mcp|rule|hook)` are all registered
  to one shared projection-walk strategy filtered by the projection's own
  agent, so no valid combination reports a false "unsupported" error.
- **Purity preserved, dry-run is inherent.** Strategies never write. A
  "dry run" is just `render.Build` (or `sync.Plan`) without `sync.Apply`,
  exactly as before — no new flag or method.

## Consequences

- `render` grows `strategy.go` (key, interface, table, `strategyFor`),
  `strategies.go` (the concrete strategies), and `reverse.go`
  (`ReverseLookup` + the owning-dir index ported from sync). `sync` loses
  ~140 lines of reverse re-derivation.
- Adding an agent or asset type is now a table edit plus, at most, one new
  strategy — the forward loop and the reverse lookup need no changes.
- `agents_doc` now honors `compatible_agents` uniformly (the old Codex arm
  bypassed the `SupportsAgent` gate that settings and generic types
  already applied). For the common empty-`compatible_agents` case the
  output is byte-identical; an asset that restricts its agents now renders
  consistently with every other type.
- A single golden test pins that every unchanged pair (all skill layouts,
  all settings, the generic projection, and `(Codex, AgentsDoc)`) is
  byte-/path-identical to the pre-refactor output; separate tests cover
  the new `CLAUDE.md` pair, the missing-pair error, the reverse
  round-trips, cursor non-invertibility, and the end-to-end Adopt through
  real `sync.Apply` + `app.Service.Apply`.
