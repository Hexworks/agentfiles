# Plan Project Screen review

Review of task 0029 (`internal/tui/shell/plan_project.go` + tests + the two retargeted parent screens). Security and Go review are clean. Clean-code, clean-architecture, SOLID, DDD, and testing reviews raised 16 issues worth deciding on. The biggest items are (1) `internal/tui/shell` now directly imports `internal/sync` even though `internal/app/service.go:173-175` declares the opposite invariant, (2) `selectProjectAssetsActions` was widened with two methods the screen does not itself call (ISP), and (3) the screen flattens the domain's two-enum drift/unknown decision space into a single screen-local `planActionState`. The rest are smaller cleanups (dead `loaded` flag, dead `prof` field, dead `actionStateOf` fallback, repeated file-row guard, dead dir-row `path` field) and missing test coverage (no `Body` rendering test, no empty-preview test, three unused error fields on the fake, tests reaching into private state).

For each issue, tick **exactly one** `[x]` solution checkbox before coming back. If every block has a chosen solution, the implementation step proceeds; otherwise the run stops.

## TUI imports `internal/sync` directly

> [!WARNING]
> Violates:
>
> - [clean_architecture.md](docs/guidelines/clean_architecture.md)
> - [tui.md](docs/guidelines/tui.md)

`internal/tui/shell/plan_project.go:16` imports `internal/sync` as `llmsync` and reads `llmsync.Preview`, `llmsync.FileChange`, `llmsync.ChangeKind`, plus `ChangeCreate`/`ChangeUpdate`/`ChangeDelete`/`ChangeDrift`/`ChangeUnknown` (`plan_project.go:28-29, 48, 70, 87, 299-309, 319, 351-353, 409-416, 445`). The explicit invariant at `internal/app/service.go:173-175` says _"the TUI consumes the app vocabulary so it never imports `internal/sync` directly, keeping the documented `tui → app` dependency edge true."_ The leak originates upstream — `internal/actions/projects.go:6,28,34` already returns `*llmsync.Preview` — but the new screen amplifies it by switching on five `llmsync.ChangeKind` constants. `select_project_assets.go:16,30-31` does the same to pass the actions handle through.

```go
// internal/tui/shell/plan_project.go
llmsync "github.com/hexworks/agentfiles/internal/sync"
// …
case llmsync.ChangeDrift:
    return []*mnemonic.Button{s.driftToggleBtn(d.path)}
case llmsync.ChangeUnknown:
    return []*mnemonic.Button{s.unknownToggleBtn(d.path)}
```

Choose one:

- [x] Mirror `Preview` / `FileChange` / `ChangeKind` in `internal/app` (next to the existing `DriftDecision` / `DriftResolution` mirrors), have `actions.PlanProject` / `SyncProject` return the app types, translate at the `actions` boundary, and drop the `llmsync` import from every file under `internal/tui/shell/`.
- [ ] Relax the invariant: update the comment at `internal/app/service.go:173-176` and ADR 0007 to say that the TUI may import `internal/sync` for read-only view types (`Preview` / `FileChange` / `ChangeKind`), while writes still go through the `app.DriftResolution` / `app.UnknownResolution` mirrors. Make the deviation explicit in docs and accept the import.
- [ ] Quarantine `llmsync` to a single `internal/tui/shell/sync_view.go` adapter file that re-exports the needed surface as `shell`-local aliases, so other screens never grow new `llmsync` references and the leak stays inventoried in one place.

## `selectProjectAssetsActions` widened with methods the screen does not use

> [!WARNING]
> Violates:
>
> - [solid.md](docs/guidelines/solid.md) — Interface Segregation
> - [clean_architecture.md](docs/guidelines/clean_architecture.md) — Common Reuse / narrow interfaces

`internal/tui/shell/select_project_assets.go:25-32` adds `PlanProject` and `SyncProject` to the parent screen's actions interface even though the parent never invokes either method — `grep` confirms the only reference inside the file is `newPlanProjectScreen(s.actions, ...)` at line 466. The two methods exist on the interface only so the parent can forward `s.actions` to the child constructor. `editProfileActions` already solves the same problem cleanly via composition (`edit_profile.go:42-46` embeds the three relevant child interfaces), keeping each constituent honest about what _its_ screen actually calls.

```go
// internal/tui/shell/select_project_assets.go:25
type selectProjectAssetsActions interface {
    LoadProfile(...)  (*profile.Profile, errs.DomainError)
    LoadProject(...)  (*project.Manifest, errs.DomainError)
    SelectAsset(...)  ([]string, errs.DomainError)
    UnselectAsset(...)([]string, errs.DomainError)
    PlanProject(...)  (*llmsync.Preview, errs.DomainError) // never called here
    SyncProject(...)  (*llmsync.Preview, errs.DomainError) // never called here
}
```

Choose one:

- [x] Compose interfaces like `edit_profile.go`: keep `selectProjectAssetsActions` narrow (Load/Load/Select/Unselect), introduce `type selectProjectAssetsAllActions interface { selectProjectAssetsActions; planProjectActions }`, and have the parent take/store the composed type so `onPlan` forwards a `planProjectActions` to the child without polluting the parent's contract.
- [ ] Split the constructor: `newSelectProjectAssetsScreen(a selectProjectAssetsActions, child planProjectActions, profileID, projectID string)` — keeps each interface honest, costs one extra argument at the two call sites.
- [ ] Accept the widening as the documented project convention and add a comment on `selectProjectAssetsActions` explaining that `PlanProject`/`SyncProject` exist only to forward to the child screen. Apply the same widening discipline anywhere a parent screen pushes another child that needs a wider action handle.

## `planActionState` flattens drift and unknown decision spaces

> [!WARNING]
> Violates:
>
> - [domain_model.md](docs/guidelines/domain_model.md) — Use The Project Language

`internal/tui/shell/plan_project.go:56-62` declares one `planActionState` enum `{planKeep, planOverwrite, planDelete}`. The domain models the same space as two distinct enums (`app.DriftDecision ∈ {DriftKeep, DriftOverwrite}` and `app.UnknownDecision ∈ {UnknownKeep, UnknownDelete}`) precisely because their valid values do not overlap (see `internal/app/service.go:179-191` and `docs/architecture/12-glossary.md`). The TUI flattens them into a third vocabulary, then re-introduces the discrimination through `ch.Kind` switches in `onApply` (`plan_project.go:408-423`) and `treeActionsFn` (`plan_project.go:350-355`). `planOverwrite` is silently drift-only and `planDelete` is silently unknown-only — a type-level invariant that lives only in coder discipline.

```go
type planActionState int
const (
    planKeep planActionState = iota
    planOverwrite // drift-only
    planDelete    // unknown-only
)
```

Choose one:

- [x] Replace `planActionState` with two typed maps: `driftResolutions map[string]app.DriftDecision`, `unknownResolutions map[string]app.UnknownDecision`. The `onApply` switch becomes two range loops; the row toggle factory picks the map by `ch.Kind`. Vocabulary aligns with the domain and the drift-only / unknown-only invariant is enforced by the type.
- [ ] Keep one map but use the domain enums directly: `resolutions map[string]any` holding either decision. Discriminate by type assertion in `onApply`. Less appealing but preserves a single store.
- [ ] Keep `planActionState` and add a doc comment cross-referencing `app.DriftDecision` / `app.UnknownDecision` plus an `// invariant: planOverwrite only on drift rows; planDelete only on unknown rows` comment so future readers don't see this as a third independent enum.

## Default-Keep policy duplicated between domain and TUI > [!WARNING]

> Violates:
>
> - [domain_model.md](docs/guidelines/domain_model.md) — Put Domain Rules In Domain Code
> - [clean_architecture.md](docs/guidelines/clean_architecture.md) — Stable policy at the center

The "absent path means Keep" rule lives in domain (`sync.DriftKeep` / `sync.UnknownKeep` are zero values, `internal/sync/sync.go:99-122`; `app.Service.Apply` defaults at `internal/app/service.go:211-212`; ADR 0010 lines 69-77) **and** in the TUI: `planKeep` is the zero value of `planActionState` (`plan_project.go:58-62`), `actionStateOf` returns `planKeep` on map miss (`plan_project.go:334-339`), and `onApply` simply omits map-absent paths from the slices it sends to the app (`plan_project.go:403-424`). The current alignment is implicit. If the domain default ever changes (or a new decision is added), the TUI silently keeps sending nothing and the meaning drifts without a compile error.

```go
func (s *planProjectScreen) actionStateOf(path string) planActionState {
    if v, ok := s.resolutions[path]; ok {
        return v
    }
    return planKeep // mirrors sync.DriftKeep / sync.UnknownKeep zero value
}
```

Choose one:

- [x] Have `onApply` emit an explicit `DriftKeep` / `UnknownKeep` resolution for every drift / unknown row by iterating `preview.Changes` rather than the map. The domain stays the single source of the default and the TUI becomes blind to what "default" means.
- [ ] Keep the optimization but lock it in with a test: `OmittedDriftPathFallsBackToDriftKeep` and `OmittedUnknownPathFallsBackToUnknownKeep`, asserted against `app.Service.Apply` behavior. Add an inline comment at `actionStateOf` referencing `internal/sync/sync.go:99-122` so the dependency is navigable.
- [ ] Accept the mirror as-is and document it as a deliberate optimization in the screen-level comment + ADR 0010 (note that the TUI relies on the domain default being `Keep`).

## Dead `loaded` field duplicates `preview != nil`

> [!WARNING]
> Violates:
>
> - [clean_code.md](docs/guidelines/clean_code.md) — Needless Repetition; Keep structs focused

`plan_project.go:98` declares `loaded bool` and `handleLoaded` sets it at line 210, but `s.preview = m.preview` at line 214 encodes the same fact. `Body` reads `!s.loaded` (line 261); `toggle` reads `s.preview != nil` (line 387); `onApply` reads `s.preview == nil` (line 398). `edit_asset.go:92-94` already documents the right pattern ("the pointer doubles as a 'loaded' sentinel"). Two encodings of one fact means a future maintainer must remember to update both.

```go
type planProjectScreen struct {
    // …
    preview *llmsync.Preview
    loaded  bool // duplicates preview != nil
}
```

Choose one:

- [x] Drop `loaded`; have every reader use `s.preview == nil` as the unloaded sentinel, matching `edit_asset.go`.
- [ ] Keep `loaded` and have `toggle` / `onApply` read it instead of `s.preview`, so there is exactly one source of truth (the opposite direction — less consistent with `edit_asset.go`).

## Dead `s.prof` field

> [!WARNING]
> Violates:
>
> - [clean_code.md](docs/guidelines/clean_code.md) — Needless Complexity; Keep structs focused

`plan_project.go:84` declares `prof *profile.Profile` and `handleLoaded` assigns to it (line 211), but the field is never read afterwards — only `s.profileName` is consumed by `Body`. Holding the whole profile pointer on the screen reads as if the rest of the screen needs it.

```go
prof        *profile.Profile // never read after assignment
profileName string
// …
s.prof = m.prof
s.profileName = m.prof.Manifest.Name
```

Choose one:

- [x] Remove the `prof` field and the assignment. Keep only `profileName`.
- [ ] Document the intent on the field if a planned future feature needs the full profile (cross-reference the task ticket).

## `actionStateOf` adds an indirection over the zero-value default

> [!WARNING]
> Violates:
>
> - [clean_code.md](docs/guidelines/clean_code.md) — Needless Complexity

`plan_project.go:332-339` returns `s.resolutions[path]` with an explicit fallback to `planKeep`. But `planKeep = iota` is already the zero value of `planActionState`, so `s.resolutions[path]` returns `planKeep` on a map miss. The `if v, ok` branch is dead.

```go
func (s *planProjectScreen) actionStateOf(path string) planActionState {
    if v, ok := s.resolutions[path]; ok {
        return v
    }
    return planKeep // map miss already returns planKeep
}
```

Choose one:

- [x] Drop the helper and replace its three callsites (`plan_project.go:322, 361, 368`) with direct `s.resolutions[path]` map reads.
- [ ] Keep the helper for the named intent but simplify to `return s.resolutions[path]` — preserves the call site readability without the dead branch.
- [ ] Accept as defensive — leave as-is in case `planKeep` is renumbered later.

## Dead `planNode.path` on directory rows

> [!WARNING]
> Violates:
>
> - [clean_code.md](docs/guidelines/clean_code.md) — Needless Complexity; Keep structs focused

`buildPlanTree` populates `path: acc` on every directory node (`plan_project.go:476`), but no reader ever reads a directory row's `path`. `statusValue` (line 295), `actionValue` (line 315), and `treeActionsFn` (line 347) all early-return when `d.kind != planNodeFile`. The dir-node `path` write is dead data.

```go
node = &treetable.Node{
    Label: part + "/",
    Data:  planNode{kind: planNodeDir, path: acc}, // path never read on dir rows
}
```

Choose one:

- [ ] Drop `path` from dir-node payloads — set it only on file leaves.
- [x] Keep it and add a comment explaining why dir rows carry a path (e.g. future per-directory bulk action).

## Repeated file-row guard across three methods

> [!WARNING]
> Violates:
>
> - [clean_code.md](docs/guidelines/clean_code.md) — Put boundary checks in one obvious place

`statusValue` (`plan_project.go:293-311`), `actionValue` (`plan_project.go:313-330`), and `treeActionsFn` (`plan_project.go:344-358`) all open with the same pattern: `d, ok := n.Data.(planNode); if !ok || d.kind != planNodeFile { return ... }`. Three callsites for one predicate.

```go
d, ok := n.Data.(planNode)
if !ok || d.kind != planNodeFile {
    return ""
}
```

Choose one:

- [x] Extract `planFileNode(n *treetable.Node) (planNode, bool)` returning `ok` only for file rows; let all three callers use it.
- [ ] Accept the three repetitions and inline a tiny `planNodeOf` helper only for the type assertion.
- [ ] Leave as-is — the duplication is small and local.

## `toggle` rebuilds entire tree on every keystroke without a code comment

> [!WARNING]
> Violates:
>
> - [clean_code.md](docs/guidelines/clean_code.md) — Comment non-obvious invariants

`plan_project.go:381-392` calls `s.tree.SetRoot(buildPlanTree(...))` on every drift/unknown toggle. It relies on the non-obvious invariant _"`treetable.SetRoot` preserves the underlying table cursor"_ — currently documented only in the changelog. A future refactor of `SetRoot` could silently break cursor stickiness. Not a correctness bug today; missing the code-level breadcrumb.

```go
if s.preview != nil {
    s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes))
}
```

Choose one:

- [ ] Add an inline comment at the `SetRoot` call referencing the cursor-preservation contract in `internal/tui/components/treetable/treetable.go`.
- [x] Expose a narrower `treetable.RefreshActions()` (or `RefreshCursorRow()`) and call it from `toggle`, leaving `SetRoot` for load-time only.
- [ ] Leave as-is; the changelog already records the rationale.

## Three unused error fields on `fakePlanActions`

> [!WARNING]
> Violates:
>
> - [testing.md](docs/guidelines/testing.md) — Cover the boundaries; drop dead scaffolding

`internal/tui/shell/plan_project_test.go:28-31` declares `loadProfErr`, `loadProjErr`, `syncErr`. The fake's `LoadProfile` / `LoadProject` / `SyncProject` respect them, but no test sets any of the three. The only load-error path covered is `planErr` (`TestPlanProjectScreen_HandleLoadedErrorEmitsNotification`). A regression that swaps the call order in `loadCmd` (`plan_project.go:172-189`) or fails to return on a `LoadProject` error would not be caught. The `SyncProject` failure path is reached only by fabricating a `mutationDoneMsg` directly (`TestPlanProjectScreen_OnApplyFailureEmitsNotificationOnly`) — that tests `handleMutationDone`, not the apply wire-up.

```go
type fakePlanActions struct {
    // …
    loadProfErr errs.DomainError // declared, never set by any test
    loadProjErr errs.DomainError // declared, never set by any test
    syncErr     errs.DomainError // declared, never reached through onApply
}
```

Choose one:

- [ ] Add three tests: `LoadProjectErrorShortCircuits`, `LoadProfileErrorShortCircuits`, `OnApplySyncErrorSurfacesAsErrorNotification` (set `f.syncErr`, drive `onApply()` end-to-end, assert the resulting `mutationDoneMsg.severity == SeverityError`).
- [x] Delete the unused fields and accept the existing coverage.

## No assertion that `Body` renders the header, tree, and button row

> [!WARNING]
> Violates:
>
> - [testing.md](docs/guidelines/testing.md) — Test One Behavior At A Time

`Body` (`plan_project.go:260-277`) has two branches — `!s.loaded` returns `" Loading…"`; loaded renders header + tree + Apply/Back button row. Neither branch is tested. `edit_profile_test.go:588-686` covers the equivalent surface. A regression where the Apply button silently disappears from the button row or the header loses the project/profile names would slip through.

Choose one:

- [x] Add `TestPlanProjectScreen_BodyPreLoadShowsLoading` (asserts `s.Body(120)` contains `"Loading"`) and `TestPlanProjectScreen_BodyLoadedContainsProjectAndButtons` (asserts `s.Body(120)` after `planLoadInto` contains `s.projectName`, `"Apply"`, `"Back"`).
- [ ] Accept the gap — `mnemonic.Set` already covers button registration, and `tree.View()` is exercised at the component level.

## No empty-preview test (zero changes)

> [!WARNING]
> Violates:
>
> - [testing.md](docs/guidelines/testing.md) — Cover the realistic paths

A no-op plan (`preview.Changes` empty or nil) is the most common steady-state case. `buildPlanTree` with no changes, `onApply` with an empty preview, `StatusKeys` with no tree buttons — none of these paths have a test. Current `onApply` calls `SyncProject` with nil `Drift` / `Unknown` slices even when there are zero changes — that may be intended or not.

Choose one:

- [x] Add `OnApplyEmptyPreviewDoesNotCallSyncProject` (early-return when `len(preview.Changes) == 0`, asserting no fake invocation) AND change `onApply` to short-circuit on an empty changes slice.
- [ ] Add `OnApplyEmptyPreviewStillCallsSyncProject` and `StatusKeysEmptyPreviewHasOnlyBack` to lock in the current behavior.
- [ ] Skip — empty-preview behavior is exercised end-to-end via the integration verification step.

## Mnemonic-uniqueness exhaustive walk has cursor off-by-one

> [!WARNING]
> Violates:
>
> - [testing.md](docs/guidelines/testing.md) — Make the safety walk actually exercise the rows

`plan_project_test.go:456-493` calls `s.rebuildSet()` _before_ dispatching `tea.KeyDown`, so the first iteration asserts uniqueness for the _root_ row (no tree button — only `{Apply, Back}`, trivially unique). The walk eventually reaches the file rows because `rowMax = len(changes) + 5`, but the coverage is "lucky" rather than designed. The candidate alphabet `{o, k, d, a, b}` is stated in the comment but never explicitly asserted per row — only uniqueness.

Choose one:

- [x] Move `s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})` to the _top_ of the loop body (after row 0) so each iteration asserts the current cursor's button set, plus add an outer assertion that at least one iteration registered a non-screen-level button (e.g. `Label == "Overwrite"`).
- [ ] Add a separate `TestPlanProjectScreen_MnemonicSetOnDriftRow` that places the cursor on a known drift row, asserts the labels are exactly `{Overwrite, Apply, Back}`, then toggles and asserts `{Keep, Apply, Back}`.
- [ ] Accept the current walk; the buffer of 5 extra rows compensates for the off-by-one.

## Tests reach into private `s.resolutions` instead of driving via UI

> [!WARNING]
> Violates:
>
> - [clean_code.md](docs/guidelines/clean_code.md) — Test behavior, not implementation
> - [testing.md](docs/guidelines/testing.md) — Assert Behavior, Not Mock Mechanics

`TestPlanProjectScreen_ActionValueReflectsResolutionMap` (line 200), `TestPlanProjectScreen_TreeActionsFnDriftOverwriteRendersKeepBtn` (line 229), `TestPlanProjectScreen_TreeActionsFnUnknownDeleteRendersKeepBtn` (line 254), and the mnemonic-uniqueness override loop (line 474) mutate `s.resolutions` directly. The user-visible way to reach the `planOverwrite` state is to press the toggle button — `TestPlanProjectScreen_ToggleDriftSwapsState` already shows the pattern. The direct-write tests would survive a refactor that replaces the map with two slices but still works correctly.

```go
s.resolutions["p"] = planOverwrite // reaches into private state
fn := s.treeActionsFn()
```

Choose one:

- [x] Drive state transitions via `btn.Trigger()` for the four named tests; keep the exhaustive mnemonic walk's direct map writes (where pressing buttons across N×M combinations would balloon the test).
- [ ] Accept the implementation coupling — the screen package owns the tests so private-state access is the project convention.

## `fakePlanActions.SyncProject` returns the plan preview on success

> [!WARNING]
> Violates:
>
> - [testing.md](docs/guidelines/testing.md) — Fakes should not hide future contract drift

`plan_project_test.go:60-61` returns `f.preview` (the _plan_) on success. The real service returns the _post-sync_ preview. The screen does not currently use the returned preview, so the discrepancy is invisible — but a future regression that starts reading the result and assumes plan-shape semantics would be hidden by the fake.

```go
func (f *fakePlanActions) SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError) {
    f.syncInputs = append(f.syncInputs, in)
    if f.syncErr != nil { return nil, f.syncErr }
    return f.preview, nil // same pointer as PlanProject — hides shape coupling
}
```

Choose one:

- [x] Add a distinct `syncResult *llmsync.Preview` field on the fake; default to an empty `&llmsync.Preview{}` in `newPlanActionsFake`. Have `SyncProject` return that instead of `f.preview`.
- [ ] Return `nil` (no preview) on success and document that the screen ignores the value.
- [ ] Accept — the fake is consistent with neighbors (`fakeSelectActions.SyncProject` does the same).
