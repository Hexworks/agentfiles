# Plan — Task 0029: Plan Project Screen

Cross-link: [description.md](./description.md)

## Context

Replace `planProjectStub` (`internal/tui/shell/plan_project_stub.go`) with a real `planProjectScreen` that loads a `Preview` via `actions.PlanProject`, renders a single treetable with two value columns (Status, Current Action) + per-row toggle button, lets the user override drift/unknown decisions, and applies via `actions.SyncProject`. Depends on tasks 0017 (ChangeUnknown), 0019 (PlanProject/SyncProject actions), 0020 (`treetable.WithValueColumns`), 0021 (TUI shell). Reuses patterns from `select_project_assets.go` (dual-load) and `edit_asset.go` (treetable + mnemonic safety walk).

## Files

**New**

- `internal/tui/shell/plan_project.go` — screen.
- `internal/tui/shell/plan_project_test.go` — tests.

**Edit**

- `internal/tui/shell/select_project_assets.go` — widen `selectProjectAssetsActions` interface; retarget `onPlan()` to push the real screen.
- `internal/tui/shell/edit_profile.go` — retarget `onPlanProject()` to push the real screen.
- `internal/tui/shell/select_project_assets_test.go` — widen `fakeSelectActions`; rename + retarget plan-push assertion.
- `internal/tui/shell/edit_profile_test.go` — rename + retarget plan-push assertion.
- `docs/architecture/05-building-block-view.md` — note Plan Project now implemented (not stub).
- `docs/architecture/06-runtime-view.md` — note `Plan → SyncProject → app.Apply → llmsync.Apply` path.

**Delete**

- `internal/tui/shell/plan_project_stub.go`.
- `internal/tui/shell/stub_screens_test.go` — only contained stub tests.

No ADR (uses established patterns).

## Step-by-step execution

### 1. Write `internal/tui/shell/plan_project.go`

Types:

```go
type planProjectActions interface {
    LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
    LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
    PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError)
    SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError)
}

type planNodeKind int
const (planNodeRoot planNodeKind = iota; planNodeDir; planNodeFile)

type planNode struct {
    kind   planNodeKind
    path   string             // forward-slash relative; "" for root
    change llmsync.FileChange // zero for dir/root
}

type planActionState int
const (planKeep planActionState = iota; planOverwrite; planDelete)

type planProjectLoadedMsg struct {
    prof    *profile.Profile
    proj    *project.Manifest
    preview *llmsync.Preview
    err     errs.DomainError
}

type planProjectScreen struct {
    actions   planProjectActions
    profileID string
    projectID string

    prof        *profile.Profile
    projectName string
    profileName string
    preview     *llmsync.Preview
    resolutions map[string]planActionState // off-default only

    tree     *treetable.Model
    applyBtn *mnemonic.Button
    backBtn  *mnemonic.Button
    set      *mnemonic.Set

    loaded bool
}
```

Constructor `newPlanProjectScreen(a planProjectActions, profileID, projectID string) *planProjectScreen` — panic guards mirror `select_project_assets.go:86-95`. Build tree, buttons, mnemonic set; `tree.Focus()`.

Treetable construction:

```go
s.tree = treetable.New(
    treetable.WithRoot(emptyPlanRoot()),
    treetable.WithNameColumn(treetable.Column{Title: "Name", Width: 40}),
    treetable.WithValueColumns(
        treetable.ValueColumn{Title: "Status", Width: 10, Value: s.statusValue},
        treetable.ValueColumn{Title: "Current Action", Width: 14, Value: s.actionValue},
    ),
    treetable.WithActions(treetable.Column{Title: "Actions", Width: 14}, s.treeActionsFn()),
    treetable.WithHeight(treetableHeight),
    treetable.WithStyles(focusAwareTreetableStyles()),
    treetable.WithTitle("Changes"),
)
```

Status value mapping (per description):
- `ChangeCreate` → `"+ add"`
- `ChangeUpdate` → `"~ update"`
- `ChangeDelete` → `"- delete"`
- `ChangeDrift` → `"* drift"`
- `ChangeUnknown` → `"? unknown"`

Current Action value: for create/update/delete return `"-"`; for drift/unknown return `"Keep"|"Overwrite"|"Delete"` via `actionStateOf(path)`.

Actions function: returns nil for root/dir/create/update/delete. For drift/unknown the button shows the **other** option:

```
drift + Keep      → [Overwrite] (o)
drift + Overwrite → [Keep] (k)
unknown + Keep    → [Delete] (d)
unknown + Delete  → [Keep] (k)
```

Toggle handler:

```go
func (s *planProjectScreen) toggle(path string, next planActionState) tea.Cmd {
    if next == planKeep { delete(s.resolutions, path) } else { s.resolutions[path] = next }
    if s.preview != nil {
        s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes))
    }
    s.rebuildSet()
    return nil
}
```

`buildPlanTree(projectName string, changes []llmsync.FileChange) *treetable.Node` — model after `buildAssetTree` (`edit_asset_helpers.go:79-119`) but split on `"/"` (forward slash), since `FileChange.Path` is forward-slash relative per `sync/sync.go:140-144` and `validatePathKey` at `sync/sync.go:394-409`.

`Init` → `loadCmd()` does three sequential calls (LoadProject, LoadProfile, PlanProject), folded into one `planProjectLoadedMsg`. Mirror `select_project_assets.go:181-196`.

`Update`:
- `planProjectLoadedMsg` → handleLoaded (set fields, `tree.SetRoot(buildPlanTree(...))`, `rebuildSet`)
- `mutationDoneMsg` → on Info severity: `tea.Batch(notificationCmd, popCmd)`; on error: notification only
- `tea.KeyPressMsg` → match against `s.set` first; otherwise forward to `s.tree.Update`; rebuild set on cursor change.

`Body(width int)`:

```
header
""
s.tree.View()
""
right-aligned [Apply] [Back]
```

`StatusKeys()` returns the cursor row's button bindings (via `s.tree.Buttons()`) + `[Back]`. **Decision pending — see Open Question below.** Default plan: include `[Back]` per existing `select_project_assets` convention.

`onApply`:

```go
func (s *planProjectScreen) onApply() tea.Cmd {
    if s.preview == nil { return nil }
    var drift []app.DriftResolution
    var unknown []app.UnknownResolution
    for _, ch := range s.preview.Changes {
        st, ok := s.resolutions[ch.Path]
        if !ok { continue }
        switch ch.Kind {
        case llmsync.ChangeDrift:
            if st == planOverwrite {
                drift = append(drift, app.DriftResolution{Path: ch.Path, Decision: app.DriftOverwrite})
            }
        case llmsync.ChangeUnknown:
            if st == planDelete {
                unknown = append(unknown, app.UnknownResolution{Path: ch.Path, Decision: app.UnknownDelete})
            }
        }
    }
    return mutationCmd(
        func() errs.DomainError {
            _, err := s.actions.SyncProject(actions.SyncProjectInput{
                ProfileRef: s.profileID, ProjectID: s.projectID, Drift: drift, Unknown: unknown,
            })
            return err
        },
        "Project synced",
    )
}
```

`rebuildSet`:

```go
func (s *planProjectScreen) rebuildSet() {
    set := mnemonic.NewSet()
    for _, b := range s.tree.Buttons() { set.Add(b) }
    set.Add(s.applyBtn)
    set.Add(s.backBtn)
    s.set = set
}
```

### 2. Edit `internal/tui/shell/select_project_assets.go`

Widen interface (lines 24-29):

```go
type selectProjectAssetsActions interface {
    LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
    LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
    SelectAsset(in actions.SelectAssetInput) ([]string, errs.DomainError)
    UnselectAsset(in actions.UnselectAssetInput) ([]string, errs.DomainError)
    PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError)
    SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError)
}
```

Add `llmsync "github.com/hexworks/agentfiles/internal/sync"` import.

Line 462-464 — `onPlan()` becomes `pushCmd(newPlanProjectScreen(s.actions, s.profileID, s.projectID))`.

### 3. Edit `internal/tui/shell/edit_profile.go`

Line 581: `pushCmd(newPlanProjectScreen(s.actions, s.profileID, p.ID))`. Interface composition already inherits the widened set.

### 4. Delete `internal/tui/shell/plan_project_stub.go`.

### 5. Delete `internal/tui/shell/stub_screens_test.go` (entire file — its only subject was the stub).

### 6. Edit `internal/tui/shell/select_project_assets_test.go`

Add fields + methods to `fakeSelectActions`:

```go
type fakeSelectActions struct {
    // existing fields...
    preview    *llmsync.Preview
    planErr    errs.DomainError
    syncErr    errs.DomainError
    syncInputs []actions.SyncProjectInput
}
func (f *fakeSelectActions) PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError) {
    return f.preview, f.planErr
}
func (f *fakeSelectActions) SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError) {
    f.syncInputs = append(f.syncInputs, in)
    return f.preview, f.syncErr
}
```

Rename `TestSelectProjectAssets_PlanPushesPlanProjectStub` → `TestSelectProjectAssets_PlanPushesPlanProjectScreen`. Replace the type assertion with `*planProjectScreen`.

### 7. Edit `internal/tui/shell/edit_profile_test.go`

Rename `TestEditProfileScreen_ProjectsPKeyPushesPlanProjectStub` → `..._PushesPlanProjectScreen`. Update type assertion to `*planProjectScreen`. Verify the fixture's actions value satisfies the wider interface (likely concrete `*actions.Actions`).

### 8. Write `internal/tui/shell/plan_project_test.go`

Test fake `fakePlanActions` implements the 4-method `planProjectActions` interface with backing fields for prof/proj/preview/errors and a `syncInputs []actions.SyncProjectInput` log. Helper `loadInto(t,s,f)` dispatches `planProjectLoadedMsg` directly.

Tests:

1. `PanicsOnInvalidConstruction` — nil actions, empty profile/project.
2. `Title` returns `"Plan Project"`.
3. `InitDispatchesLoad` — `s.Init()()` returns `planProjectLoadedMsg` populated from fake.
4. `HandleLoadedErrorEmitsNotification` — fake returns planErr; assert NotificationMsg.
5. `StatusValueForEachKind` — `+ add` / `~ update` / `- delete` / `* drift` / `? unknown`; dir/root empty.
6. `ActionValueDefaultsToKeepForDriftAndUnknown` — drift/unknown show "Keep" by default; create/update/delete show "-".
7. `ActionValueReflectsResolutionMap` — Overwrite/Delete reflected.
8. `TreeActionsFnDriftKeepRendersOverwriteBtn` — label "Overwrite", mnemonic 'o'.
9. `TreeActionsFnDriftOverwriteRendersKeepBtn` — label "Keep", mnemonic 'k'.
10. `TreeActionsFnUnknownKeepRendersDeleteBtn` — label "Delete", mnemonic 'd'.
11. `TreeActionsFnUnknownDeleteRendersKeepBtn` — label "Keep", mnemonic 'k'.
12. `TreeActionsFnNoButtonForCreateUpdateDelete` — three subcases, nil.
13. `ToggleDriftSwapsState` — trigger button; map[path]==planOverwrite; trigger again; map cleared.
14. `ToggleUnknownSwapsState` — same for unknown ↔ delete.
15. `OnApplyEmptyMapBuildsEmptySlices` — preview has changes; map empty; assert fake's `syncInputs[0].Drift == nil && .Unknown == nil`.
16. `OnApplyWithSelectionsBuildsCorrectSlices` — populate Overwrite + Delete; assert slices.
17. `OnApplySuccessEmitsNotificationAndPop` — drain `tea.Batch`; assert one NotificationMsg with "Project synced" + one PopScreenMsg.
18. `OnApplyFailureEmitsNotificationOnly` — error path; no pop.
19. `MnemonicUniquenessExhaustive` — pattern from `edit_asset_test.go:238-282`. Seed preview with one row of each `ChangeKind`. Walk cursor across all rows + buffer; for drift/unknown rows also toggle state between default and override; assert no duplicate mnemonics across `{o,k,d,a,b}`. Advance cursor via `s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})`.

### 9. Docs

- `docs/architecture/05-building-block-view.md`: update the TUI shell section to note Plan Project is implemented as a treetable-driven screen.
- `docs/architecture/06-runtime-view.md`: bullet for `Plan Project → Apply → actions.SyncProject → app.Service.Apply → llmsync.Apply` path.

No `docs/guidelines/*` additions — no new patterns.

### 10. Verify

```
make build && make test && make lint
./bin/af   # Select Project Assets → [Plan] → toggle drift/unknown rows → [Apply]
```

## Tricky bits

1. **`treetable.SetRoot` preserves cursor.** `treetable.go:233-253, 358-362` — `rebuild` rebuilds rows from current cursor, doesn't reset. Safe on every toggle.
2. **Default state encoded as map absence.** `planKeep` is zero value; only off-default entries stored. `onApply` iterates `preview.Changes` (not the map) so order is deterministic and stale entries are ignored.
3. **Button closures look up state freshly.** `makeDriftToggleBtn`/`makeUnknownToggleBtn` capture `path` + `s` pointer, not state value. The "what button is rendered" decision lives in `treeActionsFn` per render.
4. **`ActionsFunc` runs only for cursor row.** Cursor move → ActionsFunc re-runs → `tree.Buttons()` updates → `rebuildSet` picks new mnemonics.
5. **Forward-slash split.** `FileChange.Path` uses `/` (validated by `sync.validatePathKey`). `buildPlanTree` must split on `"/"` literally — NOT `filepath.Separator`. Divergence from `buildAssetTree`.
6. **Interface widening cascades.** Widening `selectProjectAssetsActions` automatically widens `editProfileActions` via composition. Real `*actions.Actions` already satisfies. Test fakes need the two new methods.

## Open question for the user

The description says: "Screen-level `a` and `b` excluded" from the status bar. The `select_project_assets` screen explicitly keeps `[Back]` in the bar "so users always have a way out" (its own comment). **Recommend: keep `[Back]` in the bar per existing convention, exclude `[Apply]`.** Confirm before implementing.

## Verification

```bash
make build && make test && make lint
./bin/af
# Welcome → Profiles → Edit Profile → select project → [Plan]
#   or
# Welcome → Profiles → Edit Profile → project row → [Select Assets] → [Plan]
# Toggle drift/unknown rows via o/k/d. Press [Apply]. Notification appears; screen pops.
```
