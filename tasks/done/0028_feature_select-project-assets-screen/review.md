# Select Project Assets screen review

The screen lands as planned: two `bubbles/table` widgets, `ctrl+1`/`ctrl+2`
focus jumps, per-row select/unselect persisting through new
`SelectAsset`/`UnselectAsset` action wrappers, and an exhaustive mnemonic
test. Build + lint + test pass. No security findings.

The substantive issues are clustered around two themes:

1. **The screen does selection state math the domain already does.** A
   local `sortAssetsByName` re-sorts the assets after `profile.AssetList()`
   already sorted them; a projected `selectedIDs` slice is computed in the
   TUI and trusted as the new persisted truth, bypassing a re-read; the
   `LoadProject` action that the plan called out is missing, so a missing
   project lands as a silent "Loading…" with no notification.
2. **Hygiene around test coverage and naming.** A loop variable in the
   exhaustive walk is dead, several multi-behavior tests pack three
   assertions under one name, the planned `service_asset_test.go` mirror
   suite was never added, and `shift+tab` is uncovered.

Plus a tail of clean-code touch-ups (dead `width`/`height` fields, awkward
`selectedSelected`/`selectedAvailable` accessors, redundant per-field
comments, a missing glossary entry for "Available Asset").

Issues, ordered by load-bearing impact:

1. [Local selection projection bypasses the persisted truth](#local-selection-projection-bypasses-the-persisted-truth)
2. [`LoadProject` missing from the actions interface; missing project silently leaves screen in `Loading…`](#loadproject-missing-from-the-actions-interface-missing-project-silently-leaves-screen-in-loading)
3. [`sortAssetsByName` duplicates `profile.AssetList`'s ordering rule](#sortassetsbyname-duplicates-profileassetlists-ordering-rule)
4. [Free-function selection helpers live in the wrong layer](#free-function-selection-helpers-live-in-the-wrong-layer)
5. [`avlExtra` loop variable in the exhaustive mnemonic test is dead](#avlextra-loop-variable-in-the-exhaustive-mnemonic-test-is-dead)
6. [Movement tests assert three behaviors under one name](#movement-tests-assert-three-behaviors-under-one-name)
7. [Planned `service_asset_test.go` mirror suite was never added](#planned-service_asset_testgo-mirror-suite-was-never-added)
8. [Screen-level idempotency and `shift+tab` cycle are uncovered](#screen-level-idempotency-and-shifttab-cycle-are-uncovered)
9. [Tests depend on alphabetical Name ordering without naming it](#tests-depend-on-alphabetical-name-ordering-without-naming-it)
10. [Dead `width` / `height` fields on the screen struct](#dead-width--height-fields-on-the-screen-struct)
11. [`selectedSelected` / `selectedAvailable` accessors read poorly](#selectedselected--selectedavailable-accessors-read-poorly)
12. [`selectCalls` vs `unselectCall` plural inconsistency](#selectcalls-vs-unselectcall-plural-inconsistency)
13. [Trailing per-field comments restate the constructor call](#trailing-per-field-comments-restate-the-constructor-call)
14. [`elasticIdx` named inconsistently with `edit_profile`'s elastic constants](#elasticidx-named-inconsistently-with-edit_profiles-elastic-constants)
15. ["Available Asset" missing from `docs/glossary.md`](#available-asset-missing-from-docsglossarymd)

---

## Local selection projection bypasses the persisted truth

> [!WARNING]
>
> - [`docs/guidelines/sync_and_safety.md`](../../../docs/guidelines/sync_and_safety.md) — "Always Plan Before Apply"
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) — Stable Dependencies / source-of-truth
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — SRP ("rendering should not also decide which assets belong to a project")

`onSelect` / `onUnselect` synchronously call the service and emit
`selectionChangedMsg{selectedIDs: <projection>}` where `<projection>` is
computed in the TUI via `appendUnique(idsOf(s.selected), a.ID)` /
`removeID(...)`. The screen then trusts that projected slice and never
re-reads the persisted manifest. Today the service is a 1:1 append/save,
so this happens to be correct; the moment the service grows any policy
(e.g. enforcing `exclusive_group` by auto-unselecting a sibling, or
normalizing the slice order) the TUI's stale projection is what the user
sees. The changelog explicitly claims _"Selection is computed from the
loaded profile, never from a live pointer the screen mutates"_ — the
implementation contradicts that by computing the next selection in the
screen.

```go
// internal/tui/shell/select_project_assets.go:425-441
func (s *selectProjectAssetsScreen) onSelect() tea.Cmd {
    a, ok := s.selectedAvailable()
    if !ok {
        return nil
    }
    return s.applySelectionChange(
        func() errs.DomainError {
            _, err := s.actions.SelectAsset(actions.SelectAssetInput{
                ProfileRef: s.profileID,
                ProjectID:  s.projectID,
                AssetID:    a.ID,
            })
            return err
        },
        appendUnique(idsOf(s.selected), a.ID),   // <-- TUI projects next state
        fmt.Sprintf("Asset %q selected", a.Name),
    )
}
```

### Solutions

- [ ] After a successful `SelectAsset` / `UnselectAsset`, have `applySelectionChange` re-issue `s.loadCmd()` (or a narrower `LoadProject`) and carry the **server-side** `SelectedAssetIDs` into `selectionChangedMsg`. Drops `appendUnique`/`removeID` entirely.
- [x] Extend `Service.SelectAsset` / `Service.UnselectAsset` to return the updated `[]string`, propagate that through the action wrappers, and use the returned slice in `selectionChangedMsg`.
- [ ] Accept the optimistic-projection UX deliberately; document in the changelog and screen comment that the TUI treats `SelectedAssetIDs` as a write-through cache and that future service-side validation will require a re-fetch.

## `LoadProject` missing from the actions interface; missing project silently leaves screen in `Loading…`

> [!WARNING]
>
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — LSP / ISP
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Understandability rule 4 ("boundary checks in one obvious place")

Plan §2 specified `selectProjectAssetsActions` would include both
`LoadProject(in actions.LoadProjectInput)` and `LoadProfile(...)`. The
implementation kept only `LoadProfile` and fishes the project out by
indexing `prof.Projects[s.projectID]`. When the lookup misses, the screen
emits `selectProjectAssetsLoadedMsg{prof: prof, proj: nil}`; `handleLoaded`
silently early-returns at lines 209-211, leaves `s.loaded = false`, and
the body is stuck on `Loading…` forever — no notification, no error path.
The `Actions.LoadProject` function already exists and returns a typed
`ProjectNotFoundError`. The plan deviation is also the reason this silent
failure mode exists.

```go
// internal/tui/shell/select_project_assets.go:26-30
type selectProjectAssetsActions interface {
    LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
    SelectAsset(in actions.SelectAssetInput) (struct{}, errs.DomainError)
    UnselectAsset(in actions.UnselectAssetInput) (struct{}, errs.DomainError)
    // plan called for: LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
}

// internal/tui/shell/select_project_assets.go:176-185
func (s *selectProjectAssetsScreen) loadCmd() tea.Cmd {
    return func() tea.Msg {
        prof, err := s.actions.LoadProfile(actions.LoadProfileInput{ProfileRef: s.profileID})
        if err != nil {
            return selectProjectAssetsLoadedMsg{err: err}
        }
        proj := prof.Projects[s.projectID]               // no typed error on miss
        return selectProjectAssetsLoadedMsg{prof: prof, proj: proj}
    }
}

// internal/tui/shell/select_project_assets.go:209-211
if m.prof == nil || m.proj == nil {
    return s, nil                                        // <-- silent dead end
}
```

### Solutions

- [x] Add `LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)` to the interface, call it alongside `LoadProfile` in `loadCmd`, and route the typed `ProjectNotFoundError` through the existing notification path. Matches the plan.
- [ ] Keep the single `LoadProfile` call but synthesize a `ProjectNotFoundError` in `loadCmd` when the map index returns nil; emit it via `selectProjectAssetsLoadedMsg{err: ...}` so the silent branch disappears.
- [ ] Minimal fix: replace the silent `m.proj == nil` branch with `return s, notificationCmd(errs.SeverityError, fmt.Sprintf("project %q not found", s.projectID))` and pop the screen.

## `sortAssetsByName` duplicates `profile.AssetList`'s ordering rule

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) — Common Closure / Common Reuse
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Code Smells rule 5 ("Needless repetition")
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — "Put Domain Rules In Domain Code"

`handleLoaded` calls `m.prof.AssetList()` (already sorted by Name via
`profile/profile.go:219`) only to feed the slice into `assetsByID`,
discarding the order. `rebuildPartition` then iterates the map (random
order) and re-sorts using the same `strings.Compare(a.Name, b.Name)` body
in a TUI-local helper. The changelog's own decision — _"`profile.AssetList()`
ordering owns the visible order — the screen does not re-sort"_ — is
contradicted by the code, and the rule has two homes that can drift apart.

```go
// internal/tui/shell/select_project_assets.go:215-249
for _, a := range m.prof.AssetList() {
    s.assetsByID[a.ID] = a                         // order lost into a map
}
...
all := make([]*asset.Asset, 0, len(s.assetsByID))
for _, a := range s.assetsByID {                   // map iteration: random
    all = append(all, a)
}
// Order matches profile.AssetList — sort by Name ascending.
sortAssetsByName(all)                              // duplicates profile.AssetList's sort

// internal/tui/shell/select_project_assets.go:480-484
func sortAssetsByName(list []*asset.Asset) {
    slices.SortFunc(list, func(a, b *asset.Asset) int {
        return strings.Compare(a.Name, b.Name)
    })
}
```

### Solutions

- [ ] Cache the already-sorted slice on the screen (`s.allAssets = m.prof.AssetList()`), partition that ordered slice in `rebuildPartition`, and drop `sortAssetsByName` entirely. `assetsByID` stays only as an O(1) lookup map.
- [ ] Iterate `m.prof.AssetList()` inside `rebuildPartition` (re-call each rebuild) instead of caching, and delete `sortAssetsByName`.
- [x] Promote the sort to `asset.SortByName` and have both `profile.AssetList` and this screen call the shared helper.

## Free-function selection helpers live in the wrong layer

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) — Stable Dependencies / Common Reuse
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Code Smells rule 5 / Design rule 4

`idsOf`, `appendUnique`, and `removeID` are package-scope free functions
inside the TUI shell. They operate on `[]string` selection ids — a domain
concern owned by `project` / `asset`. Their generic names (`appendUnique`,
`removeID`) are exactly the names another screen will accidentally reinvent
or collide with. `appendUnique` is also already covered by
`utils.Deduplicate` and `slices.Contains` + `append`; `removeID` is a one-
liner with `slices.Index` + `slices.Delete`. Either rule must move to a
domain owner, or it must collapse into stdlib usage.

```go
// internal/tui/shell/select_project_assets.go:486-510
func idsOf(list []*asset.Asset) []string {
    out := make([]string, len(list))
    for i, a := range list {
        out[i] = a.ID
    }
    return out
}

func appendUnique(ids []string, id string) []string {
    for _, existing := range ids {
        if existing == id {
            return ids
        }
    }
    return append(ids, id)
}

func removeID(ids []string, id string) []string {
    for i, existing := range ids {
        if existing == id {
            return append(ids[:i], ids[i+1:]...)   // slices.Delete equivalent
        }
    }
    return ids
}
```

### Solutions

- [ ] Delete all three and inline at the call sites: `if !slices.Contains(ids, id) { ids = append(ids, id) }` and `slices.DeleteFunc(ids, func(s string) bool { return s == id })`. Removes ~25 lines.
- [ ] Convert all three into private methods on `*selectProjectAssetsScreen` (`s.idsOfSelected()`, `s.withAdded(id)`, `s.withRemoved(id)`) so they do not pollute the `shell` package namespace.
- [x] If task 0029's Plan Project screen will need the same partition logic, lift it into `profile.Profile.PartitionAssetsForProject(projID)` and share one source of truth.
- [ ] If the solution chosen for the "Local selection projection" issue above re-reads after persistence, these helpers disappear naturally — pick a coherent combination.

## `avlExtra` loop variable in the exhaustive mnemonic test is dead

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — "Test One Behavior At A Time"
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Comments rule 4 / Code Smells rule 6

`avlExtra` iterates 0..1 but nothing in the body uses it; the only branch
referencing it (`if avlExtra == 0 { ... }`) has a comment-only body that
explicitly notes the case "is already covered by selCount=2". The two
iterations therefore run identical states, so the test silently doubles
its runtime and falsely advertises an `availableCount ∈ {0, ≥1}` matrix
that does not exist. The plan's safety walk (`focus × selectedCount ×
availableCount`) is reduced to `focus × selectedCount`.

```go
// internal/tui/shell/select_project_assets_test.go:399-413
for selCount := 0; selCount <= 2; selCount++ {
    for avlExtra := 0; avlExtra <= 1; avlExtra++ {     // unused
        assets := []*asset.Asset{a1, a2}
        selected := []string{}
        if selCount >= 1 { selected = append(selected, "a1") }
        if selCount == 2 { selected = append(selected, "a2") }
        if avlExtra == 0 {
            // shrink available to 0 by selecting everything not already selected.
            // (already covered by selCount=2)                      // comment-only body
        }
        ...
    }
}
```

### Solutions

- [x] Delete the `avlExtra` loop and the empty `if`; rely on `selCount` covering empty/non-empty available implicitly.
- [ ] Make `avlExtra` material: add an `a3` when `avlExtra == 1`, so the matrix actually has `availableCount` varying independently of `selCount`.
- [ ] Replace the nested loops with a named case table (`{"empty selected", ...}`, `{"empty available", ...}`, `{"both populated", ...}`) so each row's state is grep-able and the empty/non-empty intent is in the case name.

## Movement tests assert three behaviors under one name

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — "Test One Behavior At A Time"

`TestSelectProjectAssets_SelectMovesAssetAndCallsAction` makes three
independent promises in one function: (a) the action wrapper is called
with the right ids, (b) the command produces a `selectionChangedMsg` with
the right ids, (c) re-feeding that message into `Update` re-partitions
correctly. A failure in any one of those fails the test without telling
the reader which behavior regressed. Same problem in
`TestSelectProjectAssets_UnselectMovesAssetAndCallsAction`.

```go
// internal/tui/shell/select_project_assets_test.go:173-208
_, cmd := s.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
// (b) message-type + payload assertion
changed, ok := msg.(selectionChangedMsg)
if !slices.Contains(changed.selectedIDs, "a1") { ... }
// (a) action-wrapper invocation assertion
if len(f.selectCalls) != 1 || f.selectCalls[0].AssetID != "a1" { ... }
// (c) post-state assertion after applying the message
_, _ = s.Update(changed)
if !slices.Contains(idsOf(s.selected), "a1") { ... }
if slices.Contains(idsOf(s.available), "a1") { ... }
```

### Solutions

- [x] Split into three tests per direction: `..._SelectInvokesActionWithCursorAssetID`, `..._SelectEmitsSelectionChangedMsg`, `..._SelectionChangedMsgRebuildsPartition`. Same split for unselect.
- [ ] Keep one test per direction but use `t.Run` subtests so failure attribution shows which step regressed.
- [ ] Leave as-is; the three checks are tightly coupled to a single user gesture and re-splitting would triple the setup.

## Planned `service_asset_test.go` mirror suite was never added

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — testing pyramid; test the unit under the contract it owns

Plan §1 says: _"`internal/app/service_asset_test.go` — Mirror the same
suite at the service layer (positive + idempotent + not-found)."_
`git diff --name-only HEAD~1 HEAD` shows that file is untouched. The
service-layer contract (idempotency on duplicate / not-found semantics)
is documented in `service.go` but pinned only transitively via the actions
tests. A direct service test would make a future refactor of
`Service.SelectAsset` / `Service.UnselectAsset` independently safe.

```text
// expected in internal/app/service_asset_test.go:
TestService_SelectAsset_AddsToProject
TestService_SelectAsset_IdempotentOnDuplicate
TestService_SelectAsset_MissingAssetReturnsAssetNotFoundError
TestService_SelectAsset_MissingProjectReturnsProjectNotFoundError
TestService_UnselectAsset_RemovesFromProject
TestService_UnselectAsset_IdempotentOnNeverSelected
TestService_UnselectAsset_MissingAssetReturnsAssetNotFoundError
```

### Solutions

- [x] Add the service-layer mirror suite as planned.
- [ ] Amend the plan and changelog to record an explicit decision that the actions tests provide equivalent coverage (since they wrap the service 1:1) and the service-level mirror is intentionally omitted.

## Screen-level idempotency and `shift+tab` cycle are uncovered

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — Test the behavior under the rule, not next to it

Plan §5.C ("Idempotent select / unselect (driver fake returns nil) — no
panics on duplicate") and §5.E ("`tab` / `shift+tab` cycle") are partially
implemented at the screen layer. The screen has its own duplicate-guard
(`appendUnique` / `removeID` / `selectedSelected`/`selectedAvailable`
empty-cursor branches) that nothing exercises, and only `tab` is tested in
`TestSelectProjectAssets_FocusJumpsAndCycles`.

```go
// internal/tui/shell/select_project_assets_test.go:308-332 only covers forward tab
_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
if s.handler.Focused() != s.availableIdx { ... }
// no shift+tab follow-up
```

### Solutions

- [x] Add `TestSelectProjectAssets_SelectOnAlreadySelectedIsIdempotent` (seed `SelectedAssetIDs: []string{"a1"}`, replay the select path on `a1`, assert no panic and `len(s.selected) == 1`) and `TestSelectProjectAssets_UnselectOnEmptySelectionIsNoOp`.
- [ ] Append a `shift+tab` case to `TestSelectProjectAssets_FocusJumpsAndCycles`: `s.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})`; assert focus returns to `s.selectedIdx`.
- [ ] Table-drive the focus cycling matrix and idempotency cases (one slice of subtests for each rule).

## Tests depend on alphabetical Name ordering without naming it

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — Tests should be readable; setup that encodes a hidden assumption fails for surprising reasons
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Code Smells rule 2 (Fragility)

`TestSelectProjectAssets_LoadedSplitsSelectedAndAvailable` asserts
`available == []string{"create-task", "review-code"}` and several other
tests assume `SetCursor(0)` lands on the asset with ID `a1`. Both rely on
`profile.AssetList()` (and the screen-local `sortAssetsByName`) sorting by
`Name` ascending. If anyone changes the ordering — the changelog itself
hints at "by Type then Name" — these tests break for reasons unrelated to
the partition or movement logic they claim to verify.

```go
// internal/tui/shell/select_project_assets_test.go:131-149
if got := idsOf(s.available); !slices.Equal(got, []string{"create-task", "review-code"}) {
    t.Errorf("available = %v, want [create-task, review-code]", got)
}

// internal/tui/shell/select_project_assets_test.go:182 / 218
s.availableTable.SetCursor(0)
// assumes cursor 0 == a1 because Name="A1" sorts first
```

### Solutions

- [ ] Where the test cares about the partition split (not ordering), assert membership: `slices.Contains` + length checks, or compare sorted copies via `cmp.Diff`.
- [x] Add one dedicated `TestSelectProjectAssets_AvailableOrderingMatchesAssetList` test that pins the ordering invariant separately, so an ordering change fails exactly one test with a self-explanatory name.
- [ ] Look up the cursor target by id in movement tests (`slices.IndexFunc(s.available, ...)` → `SetCursor(idx)`) so the assertion is "select asset a1 from available", independent of position.

## Dead `width` / `height` fields on the screen struct

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Code Smells rule 4 ("Needless complexity")
> - [`docs/guidelines/tui.md`](../../../docs/guidelines/tui.md) — Natural Sizing

`s.width` and `s.height` are stored from `tea.WindowSizeMsg` but never
read. `Body(width int)` uses its parameter; nothing else consults either
field. The plan promised `(height - reserved) / 2` row sizing, but the
implementation correctly uses natural sizing (`SetHeight(len(rows)+1)`)
per `tui.md`. The fields and the message case are leftover scaffolding
from the discarded plan.

```go
// internal/tui/shell/select_project_assets.go:80
width, height int

// internal/tui/shell/select_project_assets.go:189-192
case tea.WindowSizeMsg:
    s.width = m.Width
    s.height = m.Height
    return s, nil
```

### Solutions

- [x] Delete the `width, height` fields and the `tea.WindowSizeMsg` case entirely.
- [ ] Keep the WindowSizeMsg case but drop the field writes — the message is harmless to ignore, but the dead writes should not stay.
- [ ] Decide that the plan's equal-split sizing was the intended behavior and actually apply `SetHeight((s.height - reserved) / 2)`. (Contradicts `tui.md` Natural Sizing — not recommended.)

## `selectedSelected` / `selectedAvailable` accessors read poorly

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Naming rule 1 / Naming rule 2

The names overload "selected" with two different meanings: the partition
the asset lives in and the cursor selection in the table. `s.selectedSelected()`
parses as a tautology and `s.selectedAvailable()` parses as a contradiction.

```go
// internal/tui/shell/select_project_assets.go:403-423
func (s *selectProjectAssetsScreen) selectedAvailable() (*asset.Asset, bool) { ... }
func (s *selectProjectAssetsScreen) selectedSelected() (*asset.Asset, bool)  { ... }
```

### Solutions

- [x] Rename to `availableAtCursor()` / `selectedAtCursor()` (uses the `Cursor()` term already in scope).
- [ ] Rename to `focusedAvailableAsset()` / `focusedSelectedAsset()`.
- [ ] Rename to `cursorOnAvailable()` / `cursorOnSelected()` and have them return only the asset (unwrap the `(asset, ok)` at the call site).

## `selectCalls` vs `unselectCall` plural inconsistency

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Understandability rule 2 / Naming rule 2

Two symmetric recorder slices in the fake; one plural, one singular. A
reader has to double-check that `unselectCall` is also a slice.

```go
// internal/tui/shell/select_project_assets_test.go:30-31
selectCalls  []actions.SelectAssetInput
unselectCall []actions.UnselectAssetInput
```

### Solutions

- [ ] Rename `unselectCall` to `unselectCalls`.
- [x] Rename both to `selectInputs` / `unselectInputs` (matches the input type names).

## Trailing per-field comments restate the constructor call

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Comments rule 3 ("Do not repeat what the next line of code already says")

`// [1]`, `// [2]`, `// u`, `// l`, `// p`, `// b + esc` only restate the
constructor argument used directly below. They are visual noise.

```go
// internal/tui/shell/select_project_assets.go:68-73
selectedMnemo  *mnemonic.Button // [1]
availableMnemo *mnemonic.Button // [2]
unselectBtn    *mnemonic.Button // u
selectBtn      *mnemonic.Button // l
planBtn        *mnemonic.Button // p
backBtn        *mnemonic.Button // b + esc
```

### Solutions

- [x] Delete the trailing-comment annotations.
- [ ] Replace them with one block comment above the field group naming the alphabet (`{1, 2, u, l, p, b, esc}`) so the safety contract is centralized.
- [ ] Keep `// b + esc` because the extra binding is non-obvious; drop the rest.

## `elasticIdx` named inconsistently with `edit_profile`'s elastic constants

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Naming rule 4 / Understandability rule 2

`edit_profile.go` already names the same concept `assetElasticIdx` /
`projectElasticIdx`. This screen invents a one-off `const elasticIdx = 1`
with an explanatory comment ("Index 1 is Name in
`selectProjectAssetsColumnTitles`") instead of using a consistent name.

```go
// internal/tui/shell/select_project_assets.go:328-330
// Equalize the elastic Name column across both tables so they share
// width. Index 1 is Name in selectProjectAssetsColumnTitles.
const elasticIdx = 1
equalizePanelWidth(selectedCols, availableCols, elasticIdx, elasticIdx)
```

### Solutions

- [ ] Rename to `nameColumnIdx` and drop the explanatory comment.
- [x] Rename to `selectedElasticIdx` / `availableElasticIdx` to mirror `edit_profile.go`.
- [ ] Promote a single package-level `const nameColumnIdx = 1` next to `selectProjectAssetsColumnTitles` so the source of truth is co-located.

## "Available Asset" missing from `docs/glossary.md`

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — "Use The Project Language" / "update the glossary when a new durable domain term appears"

`docs/glossary.md` defines "Selected Asset" but not "Available Asset", yet
the new screen treats the latter as a first-class partition: visible
column title, struct field name, parent-task spec term, and changelog
vocabulary. The glossary asymmetry hides the domain rule ("any asset in
the profile that is not in the project's `SelectedAssetIDs`") and any
filtering policy (e.g., that `exclusive_group` / `compatible_agents` are
not honored at selection time, only at render time).

### Solutions

- [x] Add an "Available Asset" entry to `docs/glossary.md` directly under "Selected Asset", defining the partition and noting that compatibility/exclusive-group conflicts are reported at plan time, not filtered here.
- [ ] Rename the UI label to "Unselected Assets" so "selected" / "unselected" pair naturally and only one glossary entry is needed.
- [ ] Leave as-is; argue the term is obviously the complement of "selected" and does not warrant a definition. (Not recommended — the term recurs in task 0029 too.)
