# Plan — Task 0028: Select Project Assets Screen

Cross-link: [description.md](./description.md).

## Context

Task 0028 replaces the `selectProjectAssetsStub` (`internal/tui/shell/select_project_assets_stub.go`) with the real Select Project Assets screen specified by parent task 0015. The Edit Profile screen already pushes the stub from its Projects-table `[Select Assets]` row action; the entry point exists, only the destination is a placeholder.

The screen hosts two `bubbles/table` widgets stacked vertically — Selected Assets on top, Available Assets below — and persists every row-level select/unselect immediately. That keeps the upcoming Plan Project screen (task 0029) free to plan on whatever state is current, with no batched "Save" step.

Architectural prior art:
- `edit_profile.go` — two `bubbles/table` widgets in a `focus.Handler`, row-context buttons that swap with focus, screen-level `[Back]` button.
- `edit_asset.go` — `focus.New(focus.WithModifier(focus.ModCtrl))` + `AddMnemonic(component, '1')` to wire `ctrl+1` / `ctrl+2`; `rebuildSet()` that registers only the focused row's mnemonics.
- `Service.SelectAsset` (`internal/app/service.go:530-545`) already exists. There is no symmetric `Service.UnselectAsset` (project-scoped); we add one. `actions.SelectAsset` / `actions.UnselectAsset` wrappers do not exist yet either.

### Architectural note — why NOT `UpdateProject`

Task 0028's prose says "each row-level select/unselect immediately calls `UpdateProject`." The literal `Service.UpdateProject` deliberately preserves `SelectedAssetIDs` (`service.go:511-523`, doc-comment: *"the merge contract lives here so the TUI never holds a live aggregate pointer it has half-mutated"*). Mirror that contract: single-purpose `SelectAsset` / `UnselectAsset` service methods, one per row click. The TUI passes only `assetID`, never a `*project.Manifest`. The task wording is shorthand for "persists immediately," and the architectural invariant outranks the prose.

## Step-by-step plan

### 1 — Extend the app + actions layer (new files & methods)

`internal/app/service.go`
- Add `UnselectAsset(profileRef, projectID, assetID string) errs.DomainError` directly under `SelectAsset` (~line 545). Mirror `SelectAsset` shape:
  - `resolveProject` to load the project
  - if `assetID` not present in `p.SelectedAssetIDs` → return nil (idempotent)
  - filter the slice in place, `project.Save(loaded.Root, p)`
- Idempotency matches the `SelectAsset` contract (which is a no-op if already selected).
- An `errs.DomainError` for "asset not in project's selection" is **not** needed — idempotent unselect is the friendlier UX and matches `SelectAsset`'s no-op-on-duplicate symmetry.

`internal/actions/inputs.go`
- Add `SelectAssetInput { ProfileRef, ProjectID, AssetID string }`.
- Add `UnselectAssetInput { ProfileRef, ProjectID, AssetID string }`.

`internal/actions/assets.go`
- Add `SelectAsset(in SelectAssetInput) (struct{}, errs.DomainError)` → `a.svc.SelectAsset(...)`.
- Add `UnselectAsset(in UnselectAssetInput) (struct{}, errs.DomainError)` → `a.svc.UnselectAsset(...)`.

`internal/actions/assets_test.go`
- Add `TestActions_SelectAsset_AddsToProject` — seed asset + project, call `SelectAsset`, assert `p.SelectedAssetIDs` contains the id.
- Add `TestActions_SelectAsset_IdempotentOnDuplicate` — call twice, assert single occurrence.
- Add `TestActions_UnselectAsset_RemovesFromProject`.
- Add `TestActions_UnselectAsset_IdempotentOnMissing` — calling on a never-selected asset returns nil.
- Add an unknown-asset / unknown-project error case for parity with `SelectAsset`'s `AssetNotFoundError` / `ProjectNotFoundError` semantics.

`internal/app/service_asset_test.go`
- Mirror the same suite at the service layer (positive + idempotent + not-found).

### 2 — Build the screen (replaces the stub)

Create `internal/tui/shell/select_project_assets.go` (delete `select_project_assets_stub.go`):

```go
type selectProjectAssetsActions interface {
    LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
    LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
    SelectAsset(in actions.SelectAssetInput) (struct{}, errs.DomainError)
    UnselectAsset(in actions.UnselectAssetInput) (struct{}, errs.DomainError)
}
```

State struct (`selectProjectAssetsScreen`):
- `actions selectProjectAssetsActions`
- `profileID, projectID string`
- loaded data: `project *project.Manifest`, `assetsByID map[string]*asset.Asset`, `selectedIDs []string` (mirror of `project.SelectedAssetIDs`)
- focus.Handler with `focus.WithModifier(focus.ModCtrl)`
- `selectedTable, availableTable *table.Model`
- `selectedTableMnemonic, availableTableMnemonic *mnemonic.Button` (`[1]`, `[2]`)
- per-row buttons:
  - `unselectBtn` — label `Unselect`, mnemonic `u`
  - `selectBtn`   — label `Select`,   mnemonic `l`
- screen buttons:
  - `planBtn` — label `Plan`, mnemonic `p`
  - `backBtn` — label `Back`, mnemonic `b` + extra `esc`
- `set *mnemonic.Set`
- `width, height int`

Constructor `newSelectProjectAssetsScreen(a selectProjectAssetsActions, profileID, projectID string)`:
- Panic on `nil` actions / empty ids (mirrors stub).
- Build tables: columns `ID / Name / Type / Exclusive Group / Actions`. Use `naturalColumns` + `applyTable` helpers from `edit_profile.go:409-440` (reuse — do not duplicate).
- Build buttons.
- `handler = focus.New(focus.WithModifier(focus.ModCtrl))`.
- `selectedTableMnemonic = handler.AddMnemonic(selectedTable, '1')`.
- `availableTableMnemonic = handler.AddMnemonic(availableTable, '2')`.
- `rebuildSet()`.

`Init() tea.Cmd` — return `loadCmd()` that runs `LoadProject` and (if needed) `LoadProfile` and emits a `selectProjectAssetsLoadedMsg{ project, assets, err }`. The shell does not preserve any "passed-down" profile pointer across pushes, so always fetch.

`Update`:
- `tea.WindowSizeMsg` — store width/height, recompute table widths.
- `selectProjectAssetsLoadedMsg` — store project + assets, rebuild rows, focus the Selected table.
- `mutationDoneMsg` — emit notification (reusing `mutation.go`).
- `selectionChangedMsg{ newSelectedIDs []string, info string }` — refresh local `selectedIDs`, rebuild rows, emit info notification, restore cursor to the row matching the previously-selected asset id if it still exists.
- `tea.KeyPressMsg` — let `handler` consume nav keys (tab / shift+tab / ctrl+1 / ctrl+2). If still unhandled, ask each button in the current `set` to `Matches`; first match → `Trigger()`.

Row actions:
- `onSelect()` — read the cursor's asset id from the Available table → `mutationCmd` wrapping `actions.SelectAsset(...)` → on success, batch with a `selectionChangedCmd` that re-reads the project and emits `selectionChangedMsg`.
- `onUnselect()` — same shape against `actions.UnselectAsset(...)`.
- Both should ignore clicks against an empty table (defensive: row action visible only when row exists per `rebuildSet`).

Screen buttons:
- `onPlan()` — `pushCmd(newPlanProjectStub(s.profileID, s.projectID))` (the real Plan Project Screen lands in task 0029; for now keep pushing the existing stub).
- `onBack()` — `popCmd()`.

`Body(width int) string`:
- Header line: `Selecting assets for project {{name}} ({{profile-name}})` (matches the layout box in the task description).
- Above selected table: render the `[1]` mnemonic chip via `selectedTableMnemonic.View()`; same for `[2]` above the available table.
- Render selected table.
- Render available table.
- Right-aligned button row: `[Plan] [Back]` (use `lipgloss.PlaceHorizontal(..., lipgloss.Right, ...)` — same pattern as `backOnlyScreenBase.renderBody`).
- Sizing per task spec: "Heading + button row + notification area + status bar fixed; the two tables split the remaining space equally." Implementation: each table gets `(height - reserved) / 2` rows where reserved = heading (1) + 2 table-title rows + 1 button row + spacers. The shell already places the notification area + status bar outside the body. If `height == 0` (pre-WindowSize), fall back to a tiny `(8, 8)` so `bubbles/table` does not degrade (cf. `modalSize` floor in `screen.go:85-108`).

`Title() string` — `"Select Project Assets"`.

`StatusKeys() []key.Binding`:
- Always include the global keys. Append the **row-context** mnemonic of the focused table only:
  - Selected focused, row present → `[u]`
  - Available focused, row present → `[l]`
- Per `screen.go:27-32` doc + tui.md: screen-level buttons (`[Plan]`, `[Back]`) MUST NOT appear in the status bar.

`InputFocused() bool` — `false`. No huh inputs on this screen.

`rebuildSet()`:
- Always register `selectedTableMnemonic` and `availableTableMnemonic`.
- If selected table focused AND `len(selectedIDs) > 0` → register `unselectBtn`.
- If available table focused AND `len(available) > 0` → register `selectBtn`.
- Always register `planBtn`, `backBtn`.
- Candidate alphabet `{1, 2, u, l, p, b}` — pairwise unique by construction.

Row-building helpers:
- `buildSelectedRows()` — iterate `selectedIDs`, look up asset by id, materialize `table.Row{a.ID, a.Name, string(a.Type), a.ExclusiveGroup, cell}` where `cell` is `unselectBtn.View()` for the cursor row and `""` otherwise (mirrors `edit_profile.go:475-477`).
- `buildAvailableRows()` — derive `available = profile.AssetList() − selectedIDs`, stable-sorted (re-use `utils.Deduplicate` and existing `profile.AssetList()` ordering at `profile.go:214-223`). Cell shows `selectBtn.View()` for the cursor row.

### 3 — Wire the screen in

`internal/tui/shell/edit_profile.go`
- Widen `editProfileActions` to include `selectProjectAssetsActions`. Use the same composition idiom as `editProfileActions` already does for `editAssetActions` (line 41-44).
- `onSelectAssets()` (line 566-572): change the push from `newSelectProjectAssetsStub(...)` to `newSelectProjectAssetsScreen(s.actions, s.profileID, p.ID)`.

`internal/tui/shell/edit_profile_test.go`
- `TestEditProfileScreen_ProjectsAKeyPushesSelectProjectAssetsStub` (line 370) — rename and assert push of `*selectProjectAssetsScreen`. Same fake-actions fixture; widen the fake to satisfy the new interface methods (add `SelectAsset`, `UnselectAsset`, `LoadProject` stubs returning `nil, nil`).

### 4 — Delete the stub

- Delete `internal/tui/shell/select_project_assets_stub.go`.
- Update `internal/tui/shell/stub_screens_test.go`:
  - Remove the `select project assets` case from `TestNavigationStubs_SharedBackBehavior` (lines 27-34).
  - Remove the two `selectProjectAssetsStub` entries from `TestNavigationStubs_EmptyIDsPanic` (lines 99-100).
  - Remove `newSelectProjectAssetsStub("a", "b")` from `TestNavigationStubs_BodyHasNaturalHeight` (line 119).
  - Remove the `_ Screen = (*selectProjectAssetsStub)(nil)` compile-time guard (line 133).
  - Plan Project stub remains intact.

### 5 — Tests for the new screen

Create `internal/tui/shell/select_project_assets_test.go`. Mirror the shape of `edit_asset_test.go` / `edit_profile_test.go`:

A. **Mnemonic uniqueness exhaustive walk** — *the safety rule*:
```
focus ∈ {selectedTable, availableTable}
selectedCount ∈ {0, ≥1}   // toggled by seeding selectedIDs
availableCount ∈ {0, ≥1}
```
For each combination, call `rebuildSet()` and assert no two registered buttons share a `Key()`. Walk the cursor down every row of the focused table. Reuse `assertUniqueMnemonics` shape from `edit_asset_test.go:284-295`.

B. **Status-bar contract**:
- Selected focused + rows present → `StatusKeys` contains `u`, does NOT contain `p`, `b`, `l`.
- Available focused + rows present → `StatusKeys` contains `l`, does NOT contain `p`, `b`, `u`.
- Empty focused table → `StatusKeys` contains only the global bindings (no row mnemonic).

C. **Round-trip behavior**:
- `select` action: cursor on an available row, press `l`, assert fake `SelectAsset` invoked with right ids, simulate `selectionChangedMsg`, assert the row moved Available → Selected.
- `unselect` action: symmetric.
- Idempotent select / unselect (driver fake returns nil) — no panics on duplicate.

D. **Push contracts**:
- `p` (Plan) emits `PushScreenMsg` whose `Screen` is `*planProjectStub` (rename to `*planProjectScreen` after task 0029).
- `b` and `esc` both emit `PopScreenMsg`.

E. **Focus jumps**:
- `ctrl+1` focuses selectedTable.
- `ctrl+2` focuses availableTable.
- `tab` / `shift+tab` cycle.

F. **Load error path**:
- `LoadProject` returns `ProjectNotFoundError` → screen emits a notification of matching severity, body still renders without panic.

G. **Compile-time guard**: `_ Screen = (*selectProjectAssetsScreen)(nil)`.

### 6 — Documentation

- No new ADR required: this is a screen implementation that fits cleanly under ADR 0011 (TUI screen router). Touching `Service.UnselectAsset` is a small symmetric extension of `SelectAsset` and does not warrant an ADR.
- Add a one-line entry to `docs/architecture/05-building-block-view.md` *only* if the existing screen catalog enumerates Edit Asset/Edit Profile — match whichever shape is already there. (If the doc doesn't enumerate per-screen, skip.)
- Update `docs/glossary.md` only if "Selected Assets" or "Available Assets" terms are not already defined.
- No new `docs/guidelines/` entry.

### 7 — Changelog (post-implementation)

`docs/changelog/2026-06-12_0028-select-project-assets-screen.md` — fill in from `./changelog-template.md`.

## Critical files modified

| File | Change |
|---|---|
| `internal/app/service.go` | Add `Service.UnselectAsset` |
| `internal/app/service_asset_test.go` | Add tests for `UnselectAsset` (+ idempotency / not-found) |
| `internal/actions/inputs.go` | Add `SelectAssetInput`, `UnselectAssetInput` |
| `internal/actions/assets.go` | Add `Actions.SelectAsset`, `Actions.UnselectAsset` |
| `internal/actions/assets_test.go` | Tests for the two action wrappers |
| `internal/tui/shell/select_project_assets.go` | **new** — replaces the stub |
| `internal/tui/shell/select_project_assets_test.go` | **new** — exhaustive mnemonic walk + behavior |
| `internal/tui/shell/select_project_assets_stub.go` | **deleted** |
| `internal/tui/shell/stub_screens_test.go` | Remove select-project-assets cases |
| `internal/tui/shell/edit_profile.go` | `onSelectAssets` pushes real screen; widen action interface |
| `internal/tui/shell/edit_profile_test.go` | Rename test + widen fake-actions |

## Reused utilities (do not duplicate)

- `mnemonic.New`, `mnemonic.NewSet` (`internal/tui/components/mnemonic/`).
- `focus.New(focus.WithModifier(focus.ModCtrl))`, `handler.AddMnemonic`, `handler.Add`, `handler.Focused`, `handler.FocusIndex` (`internal/tui/components/focus/`).
- `mutationCmd`, `mutationDoneMsg` (`shell/mutation.go`).
- `notificationCmd` (`shell/notifications.go`).
- `naturalColumns`, `tableNaturalWidth`, `applyTable`, `equalizePanelWidth` (`shell/cellrender.go`, `edit_profile.go:409-440`).
- `pushCmd`, `popCmd` (`shell/screen.go:72-79`).
- `profile.AssetList()` (`profile.go:214-223`) for stable available-asset ordering.

## Verification

```
make fmt
make build
make test
make lint
./bin/af
  → Welcome → Profiles → row [Edit] → Projects row [Select Assets]
  → assert two tables, ctrl+1 / ctrl+2 focus, l selects, u unselects,
    p pushes Plan stub, b pops back, esc pops back.
  → reopen the screen and confirm the prior selection persisted.
```

Manual UI smoke is mandatory per the task body's `Verification` block. The mnemonic-uniqueness exhaustive test is the regression gate per the safety rule in task 0015.
