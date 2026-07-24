# Plan — Task 0011: per-(Agent,Type) render/reverse strategy table

Task: [`./description.md`](./description.md)
Relevant ADR: [`../../../docs/adr/0020-adopt-drift-into-profile.md`](../../../docs/adr/0020-adopt-drift-into-profile.md) (Adopt is why forward+reverse must live together)
Sibling precedent: task 0010 (`asset.Init` → strategy) — strategy pattern, in-package.

## Goal

Replace the `a.Type` switch in `render.addRenderedFilesFor`
(`render.go:137`) + the per-agent branching in `addSkillOutputs`
(`render.go:211`) with a **global lookup table** of strategies keyed by a
typed `(agent.Agent, asset.Type)` pair. Each strategy is a small,
self-contained implementation that knows only two things: `render`
(forward) and `reverse` (back, for Adopt). `sync`'s reverse
re-derivation (`assetProjectionDirs` `sync.go:1001`,
`owningAssetSourceRelFor` `sync.go:1083`) is deleted and replaced by
delegation to `render`, so forward and reverse cannot drift.

This is a **refactor + one behavior change**: per-agent `agents_doc`
(claude-code → `CLAUDE.md`, other agents → `AGENTS.md`) is added (today
`agents_doc` renders only for Codex). Everything else stays byte- and
path-identical.

## Design decisions (from clarification)

1. **One strategy per `(Agent, Type)` pair.** No shared "skill vs cursor"
   branching by name — `(Cursor, Skill)` is just another table entry.
   Strategies may share an underlying helper/parameterized struct when
   their logic is identical (the three skill-folder agents; the twelve
   generic-projection pairs), but each is registered explicitly under its
   own key. The dispatch is a **map lookup, never a switch**.

2. **Typed key, enum-like.** `type strategyKey struct { Agent agent.Agent; Type asset.Type }`.
   Both `agent.Agent` and `asset.Type` are already `type X string` with a
   fixed constant set — no bare strings in the table.

3. **Missing entry → aggregated `DomainError`.** A new
   `UnsupportedRenderingError{Agent, Type}` (in `render/errors.go`) with
   `Error()` → `"missing render strategy for <Agent> <Type>"` (e.g.
   "missing render strategy for cursor skill"). `Build` accumulates it via
   the existing `errs`-slice pattern (`errors.Join`-style), never
   short-circuits.

4. **Strategies are pure — dry-run is inherent.** `render` reads source
   asset files (input) and returns `[]RenderedFile` (paths + bodies in
   memory); it **never writes** the target repo. Writing stays solely in
   `sync.Apply` / `writeRendered` (`sync.go:805`). So "dry run" = call
   `render.Build` (or `sync.Plan`) and don't call `sync.Apply`, exactly as
   today. No dry-run flag or extra method is added. Preserves Invariant #1
   (plan before apply) and I/O-at-edges.

5. **Both directions in the strategy.** Interface:
   ```go
   type Strategy interface {
       // Render emits the files this (agent,type) produces for one asset.
       // Pure: reads asset sources, writes nothing.
       Render(a *asset.Asset, ag agent.Agent) ([]RenderedFile, []errs.DomainError)

       // Reverse maps a managed repo path back to the asset-relative
       // source path for Adopt. ok=false when the mapping is lossy
       // (cursor skill flat-file) — the strategy is the authority on
       // its own invertibility.
       Reverse(ctx ReverseContext) (sourceRel string, ok bool)
   }
   ```
   `ReverseContext` carries the repo path plus the owning dir's
   `TargetRoot` / `SourceRoot` / `AssetID` (supplied by `ReverseLookup`,
   below). Non-lossy strategies implement `Reverse` as the
   suffix-preserving inverse of their `Render`; `(Cursor, Skill)` returns
   `ok=false`.

6. **`agents_doc` per-agent, in scope.** `(ClaudeCode, AgentsDoc)` →
   `CLAUDE.md`; `(Codex|Cursor|OpenCode, AgentsDoc)` → `AGENTS.md`. Add
   `"CLAUDE.md"` to the managed-surface fence (`surfaces.go:22`) and a
   `config.ClaudeDocFileName = "CLAUDE.md"` constant next to
   `AgentsDocStarterFileName` (`config/paths.go:70`).

7. **Reverse entry point on the plan.** `func (p *ProjectPlan) ReverseLookup(repoPath string) (assetID, sourceRel string, ok bool)`.
   It (a) finds the owning rendered dir for `repoPath` from the plan's own
   `Files` (exact hit or parent-dir walk, the job `assetProjectionDirs`
   does today), (b) reads the producing strategy from the owning file's
   provenance, (c) delegates the tail→sourceRel decision to
   `strategy.Reverse`. `sync` calls `plan.ReverseLookup(path)` at both
   sites (`sync.go:321`, `sync.go:682`). To carry provenance, add
   `Agent agent.Agent` and `Type asset.Type` to `RenderedFile` (alongside
   `AssetID`/`SourceRel`); this also lets the preview group by agent later.

## Strategy table (24 keys)

| Type \ Agent | codex | claude-code | cursor | opencode |
|---|---|---|---|---|
| **skill** | `.codex/skills/<id>/<rel>` | `.claude/skills/<id>/<rel>` | `.cursor/commands/<id>.md` *(reverse: ok=false)* | `.opencode/skills/<id>/<rel>` |
| **agents_doc** | `AGENTS.md` | `CLAUDE.md` **(new)** | `AGENTS.md` **(new)** | `AGENTS.md` **(new)** |
| **settings** | `.codex/config.toml` | `.claude/settings.local.json` | `.cursor/config.json` | `.opencode/config.json` |
| **mcp/rule/hook** | manifest `projection.Target` (per-agent projection), shared projection-walk helper | | | |

Sources: skill roots `surfaces.go:88`; cursor `surfaces.go:80`; settings
`agent.Descriptors()` `agent.go:118`; generic projections `render.go:172`.

## Execution steps

1. **`config/paths.go`** — add `ClaudeDocFileName = "CLAUDE.md"`.
2. **`surfaces.go:22`** — add `"CLAUDE.md"` to `roots`.
3. **`render/errors.go`** — add `UnsupportedRenderingError{Agent, Type}`
   with `Error()`.
4. **`render/strategy.go`** — `strategyKey`, `Strategy` interface,
   `ReverseContext`, the global `registry map[strategyKey]Strategy` (built
   once), and `strategyFor(ag, t) (Strategy, bool)`.
5. **`render/strategies.go`** — concrete strategies, porting each current
   switch arm verbatim (except agents_doc, which gains per-agent target):
   - skill-folder (codex/claude/opencode) — shared struct parameterized by
     `surfaces.SkillRoot`; from `addSkillOutputs` (`render.go:225`).
   - cursor-skill — flat file; `Reverse` → `ok=false`; from `render.go:237`.
   - agents_doc — per-agent target via `config.ClaudeDocFileName` /
     `AgentsDocStarterFileName`; **all four agents** now.
   - settings — one per agent, driven by `agent.Descriptors()`; from
     `render.go:151`.
   - generic-projection (mcp/rule/hook × all agents) — shared helper over
     `a.Projections` filtered by the projection's agent, incl. dir walk;
     from `render.go:172` + `walkProjection`.
6. **`render/render.go`** — rewrite `Build`'s per-asset loop to iterate
   enabled+supported agents, `strategyFor(ag, a.Type)` → `Render`,
   accumulate files+errors; append `UnsupportedRenderingError` on miss.
   Delete `addRenderedFilesFor`, `addSkillOutputs`. Keep `readAssetFile`,
   `classifyFileError`, `walkProjection` as shared helpers. Add `Agent`,
   `Type` to `RenderedFile`.
7. **`render/reverse.go`** — `ProjectPlan.ReverseLookup` + owning-dir
   index (port `assetProjectionDirs` + `stripCommonSuffix` here) +
   per-strategy `Reverse`.
8. **`sync/sync.go`** — replace both call sites with
   `plan.ReverseLookup(path)`; delete `assetProjectionDirs`,
   `owningAssetSourceRelFor`, `assetDirEntry`, `stripCommonSuffix`, and the
   apply-loop `assetDirs` field (`sync.go:519`). Confirm `DriftAdopt`
   (reads AssetID/SourceRel from prior state, `sync.go:632`) is untouched.
9. **Tests** — see Acceptance Criteria.
10. **Docs** — remove the stale "Task 0011 tracks…" comment
    (`render.go:134`); update `sync.go` reverse comments to point at
    `render.ProjectPlan.ReverseLookup`.

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Forward dispatch is `switch a.Type` | `internal/render/render.go:137` | `switch a.Type {` |
| Skill per-agent branching lives in `addSkillOutputs` | `internal/render/render.go:211` | `func addSkillOutputs(files map[string]RenderedFile, a *asset.Asset, enabledAgents []agent.Agent) []errs.DomainError {` |
| `agents_doc` today renders **only** Codex → AGENTS.md | `internal/render/render.go:141` | `if !slices.Contains(enabledAgents, agent.Codex) { return nil }` |
| AGENTS.md is the only agents_doc target constant; no CLAUDE.md | `internal/config/paths.go:70` | `const AgentsDocStarterFileName = "AGENTS.md"` |
| Managed-surface fence lists AGENTS.md, not CLAUDE.md | `internal/surfaces/surfaces.go:22` | `var roots = []string{ "AGENTS.md", ".claude", ... }` |
| Skill folder roots per agent | `internal/surfaces/surfaces.go:88` | `".codex/skills", ".claude/skills", ".opencode/skills"` |
| Cursor skill flattens to one `.md`, dropping filename | `internal/render/render.go:238` | `target := filepath.ToSlash(filepath.Join(surfaces.CursorCommandsRoot(), a.ID+".md"))` |
| Settings source/target pairs from `agent.Descriptors()` | `internal/agent/agent.go:118` | `{Codex, "codex.toml", ".codex/config.toml"}, ...` |
| Generic types use manifest `projections` with per-projection agent | `internal/render/render.go:174` | `for _, projection := range a.Projections { if !slices.Contains(enabledAgents, projection.Agent) {` |
| `RenderedFile.SourceRel` is the reverse key | `internal/render/render.go:39` | `SourceRel string` |
| Reverse is re-derived in sync from the plan today | `internal/sync/sync.go:1001` | `func assetProjectionDirs(files []render.RenderedFile) map[string]assetDirEntry {` |
| Both reverse call sites route through `owningAssetSourceRelFor` | `internal/sync/sync.go:321`, `:682` | `owningAssetSourceRelFor(pth, assetDirs)` / `owningAssetSourceRelFor(change.Path, a.assetDirs)` |
| DriftAdopt reads AssetID/SourceRel from prior state (not the reverse walk) | `internal/sync/sync.go:632` | `if !ok \|\| rendered.AssetID != prior.AssetID \|\| rendered.SourceRel != prior.SourceRel {` |
| Writes happen only in sync's `writeRendered` | `internal/sync/sync.go:805` | `f, err := os.OpenFile(abs, os.O_WRONLY\|os.O_CREATE\|os.O_TRUNC\|safeWriteFlags, file.Mode)` |
| Plan/preview already renders bodies (dry-run exists structurally) | `internal/sync/sync.go:286` | `rendered, renderErrs := render.Build(p, proj)` |

## ADRs / docs to touch

> [!IMPORTANT]
> Rendering is one of the most important parts of the whole application.
> This change **must** be documented with a new ADR **and** diagrams. These
> are first-class deliverables, not optional follow-up:
>
> - **New ADR `docs/adr/0021-*.md`** — records the render/reverse strategy
>   model: the `(Agent, Type)` typed key, the global lookup table,
>   both-directions-in-one-unit isomorphism (Adopt), `UnsupportedRenderingError`
>   on a missing pair, per-agent `agents_doc` (CLAUDE.md for claude-code) +
>   the `CLAUDE.md` fence widening, and strategy purity (dry-run = render
>   without apply). Supersedes the reverse-mapping mechanics noted in ADR 0020.
> - **Component diagram** in `docs/architecture/05-building-block-view.md`
>   (mermaid) — render's internals: `Build` → strategy table (keyed by
>   `(Agent,Type)`) → `RenderedFile`; `sync` → `render.ProjectPlan.ReverseLookup`.
>   Show that `sync.Apply` is the only writer.
> - **Sequence diagram** in `docs/architecture/06-runtime-view.md` (mermaid) —
>   forward: `app.Service.Plan` → `sync.Plan` → `render.Build` →
>   `strategyFor(agent,type).Render` → hash/classify → `Preview`.
> - **Workflow/reverse sequence diagram** in `06-runtime-view.md` (mermaid) —
>   Adopt: `sync.Apply` → `ProjectPlan.ReverseLookup` →
>   `strategyFor(agent,type).Reverse` → `AdoptRequest` → `app.Service.Apply`
>   writes back into the profile asset.

- **ADR 0020** — add a pointer note: reverse mapping is now owned by
  `render.ProjectPlan.ReverseLookup`; superseded-by 0021 for the mechanics.
- **`CLAUDE.md`** — update `render` (owns the isomorphic (Agent,Type)
  strategy table + reverse) and `sync` (delegates reverse) descriptions;
  update the `surfaces` fence list to mention CLAUDE.md.
- **`docs/architecture/`** — beyond the new diagrams, repoint any
  building-block prose naming `assetProjectionDirs`/`owningAssetSourceRelFor`
  to `ReverseLookup`.
- **`docs/changelog/`** — implement skill writes the dated entry.
- No `docs/guidelines/` changes.

## Acceptance Criteria

(Mirror of `description.md`; the implement skill's DoD gate enforces these.)

- [ ] `make fmt && make test && make lint && make build` all pass.
- [ ] `TestBuild_UnchangedPairs_Golden`: a fixture profile rendered across
      all agents produces `[]RenderedFile` byte-/path-identical to a golden
      captured from the **pre-refactor** `Build` for every pair whose
      behavior is unchanged (all skill, all settings, all generic, and
      `(Codex, AgentsDoc)`).
- [ ] `TestBuild_AgentsDoc_PerAgentTargets`: an `agents_doc` asset enabled
      for all four agents renders `CLAUDE.md` (claude-code) and `AGENTS.md`
      (codex, cursor, opencode) — the new behavior — asserted on
      `t.TempDir()` output paths + bodies.
- [ ] `TestSurfaces_AllowsClaudeMd`: `surfaces.IsAllowed("CLAUDE.md")` is
      true; a full `render.Build`→`sync.Plan` over a claude-code project
      surfaces the `CLAUDE.md` change (not a fence rejection).
- [ ] `TestStrategyFor_MissingPair_AggregatesUnsupportedError`: looking up
      an unregistered `(Agent, Type)` returns `ok=false`, and `Build`
      accumulates a single `UnsupportedRenderingError` (joined, not
      short-circuited) whose `Error()` names the agent and type.
- [ ] `TestReverse_SkillFolderAgents_RoundTrip`: for codex/claude-code/
      opencode, a multi-file skill rendered into `t.TempDir()` then
      `plan.ReverseLookup`'d recovers the exact `(assetID, sourceRel)` for
      every file, including an untracked sibling file in the skill dir.
- [ ] `TestReverse_CursorSkill_NotInvertible`: `(Cursor, Skill).Reverse`
      returns `ok=false`; a real `sync.Plan`/`Apply` over a repo with that
      file offers **no** Adopt row for it.
- [ ] `TestReverse_Settings_And_AgentsDoc_RoundTrip`: settings reverse to
      the descriptor source per agent; `CLAUDE.md`→`(id, AGENTS.md source)`
      and `AGENTS.md`→same source resolve correctly.
- [ ] `TestReverse_GenericProjection_RoundTrip`: mcp/rule/hook projections
      (incl. a walked directory) reverse to source via `ReverseLookup`.
- [ ] `TestAdopt_SkillDrift_EndToEnd`: real `sync.Apply` over a drifted
      managed skill file returns an `AdoptRequest` whose `(AssetID,
      SourceRel)` equals `ReverseLookup`, and `app.Service.Apply` writes
      the local body back to `assets/skill/<id>/<rel>` — asserted by
      reading the profile asset file off disk after Apply.
- [ ] `grep -n "switch" internal/render/render.go` shows no `a.Type`
      dispatch switch; `assetProjectionDirs`/`owningAssetSourceRelFor` are
      deleted from `internal/sync/sync.go` (reverse delegates to
      `render.ProjectPlan.ReverseLookup`).
- [ ] `docs/adr/0021-*.md` exists and records the render/reverse strategy
      model (key, table, isomorphism, UnsupportedRenderingError, per-agent
      agents_doc + fence widening, strategy purity).
- [ ] `docs/architecture/05-building-block-view.md` gains a mermaid
      component diagram of render (Build → strategy table → RenderedFile;
      sync → ReverseLookup; sync.Apply the sole writer).
- [ ] `docs/architecture/06-runtime-view.md` gains two mermaid sequence
      diagrams: forward render (Plan→Build→Render→Preview) and reverse
      Adopt (Apply→ReverseLookup→Reverse→AdoptRequest→write-back).

## Risks

- **Generic reverse needs plan context.** Skill/settings/agents_doc
  reverse is structural; generic projection targets are arbitrary, so
  `ReverseLookup` consults the plan's rendered dirs (port the existing
  suffix-strip). Mitigated by moving that logic wholesale into `reverse.go`.
- **Golden brittleness / behavior change.** Capture the golden from the
  current `Build` before touching code; split assertions into
  unchanged-pairs (byte-identical) vs the new agents_doc pairs.
- **Silent Adopt regression.** The end-to-end Adopt test through real
  `sync.Apply` + `app.Service.Apply` is the guard, not the unit round-trip.
- **Fence widening.** Adding `CLAUDE.md` widens the safety fence — covered
  by `TestSurfaces_AllowsClaudeMd` and the existing traversal-rejection
  tests must still pass unchanged.
