# Task 0026 — Edit Profile Screen

Linked task: [`./description.md`](./description.md) (in
`tasks/current/0026_feature_edit-profile-screen/`).

## Context

Task 0025 shipped the Profiles screen with an `editProfileStub` placeholder
so the row-level `[Edit]` action had a valid push target. Task 0026 replaces
that stub with the real Edit Profile screen: a two-table view (Assets +
Projects) with focus mnemonics (`[1]` / `[2]`), row context actions
(`e`/`d` on Assets; `e`/`a`/`p`/`d` on Projects), and screen-level
buttons (`[c Create Asset]`, `[r Register Project]`, `[b Back]`). The
screen wires the existing actions (`LoadProfile`, `CreateAsset`,
`DeleteAsset`, `RegisterProject` via `AddProject`, `UpdateProject`,
`DeleteProject`) and modals (Create Asset, Edit Project, Register
Project, Confirmation) shipped in tasks 0018–0023.

Out of scope per the task: Edit Asset (0027), Select Project Assets
(0028), and Plan Project (0029) screen bodies. To keep the call sites
real, this task ships three navigation stubs for those, mirroring the
`editProfileStub` pattern from task 0025.

## Files Touched

| Path | What |
| ---- | ---- |
| `internal/tui/shell/edit_profile.go` (NEW) | Real `editProfileScreen` Screen impl. |
| `internal/tui/shell/edit_profile_test.go` (NEW) | Unit tests (mnemonic uniqueness matrix, focus cycle, row actions, screen actions, body height, delete-project-files-untouched). |
| `internal/tui/shell/edit_profile_stub.go` (DELETE) | Replaced by the real screen. |
| `internal/tui/shell/edit_profile_stub_test.go` (DELETE) | Same. |
| `internal/tui/shell/profiles.go` | Swap `newEditProfileStub(p.Manifest.ID)` → `newEditProfileScreen(s.actions, p.Manifest.ID)`. |
| `internal/tui/shell/profiles_test.go` | Update `TestProfilesScreen_EKeyPushesEditProfileStub` to expect `*editProfileScreen` and assert it carries the profile id. |
| `internal/tui/shell/edit_asset_stub.go` (NEW) | Push target for assets-table `[Edit]` (task 0027). |
| `internal/tui/shell/edit_asset_stub_test.go` (NEW) | |
| `internal/tui/shell/select_project_assets_stub.go` (NEW) | Push target for projects-table `[a select Assets]` (task 0028). |
| `internal/tui/shell/select_project_assets_stub_test.go` (NEW) | |
| `internal/tui/shell/plan_project_stub.go` (NEW) | Push target for projects-table `[p Plan]` (task 0029). |
| `internal/tui/shell/plan_project_stub_test.go` (NEW) | |
| `docs/changelog/2026-06-11_0026-edit-profile-screen.md` (NEW) | Changelog entry. |

No ADR or guideline changes. The TUI router (ADR 0011), the
profile-as-source-of-truth invariant (ADR 0001), and the existing
TUI / clean-architecture / errors / domain-model / testing guidelines
already cover everything this screen does. No new architectural decision
is introduced.

## Existing Code This Reuses

- `internal/tui/components/focus` — `focus.New(focus.WithModifier(focus.ModCtrl))`, `AddMnemonic`, `Update` (handles `tab`/`shift+tab`/`ctrl+<digit>`), `FocusedComponent`, `FocusIndex`, `Close`.
- `internal/tui/components/mnemonic` — `New`, `WithExtraBindingKeys`, `Set`, `Set.Add`, `Set.Match`.
- `internal/tui/components/modal` — `NewConfirm`, `NewForm`, `Modal.Init/Update/SetSize/Render`, `ResolvedMsg`.
- `internal/tui/modals` — `NewCreateAsset(asset.Manifest{})`, `NewEditProject(modals.EditProjectInput{...})`, `NewRegisterProject(modals.RegisterProjectInput{})`.
- `internal/actions` — `LoadProfile`, `CreateAsset`, `DeleteAsset`, `AddProject`, `UpdateProject`, `DeleteProject` (input structs in `inputs.go`).
- `internal/tui/notifications` — `NotificationMsg`.
- `internal/tui/shell` — `Screen` interface, `pushCmd`, `popCmd`, `modalSize`, `globalKeyMap`, `mutationCmd` (generic, in `profiles.go` — visible package-wide), `notificationCmd` (same).

The mutation helpers in `profiles.go` are package-private and already
visible to the new file; reuse them rather than duplicating.

## Screen Design

### Struct

```go
type editProfileScreen struct {
    actions   *actions.Actions
    profileID string

    prof *profile.Profile
    // ordered, stable snapshots so tests don't depend on map iteration.
    assetsList   []*asset.Asset
    projectsList []*project.Manifest

    handler        *focus.Handler
    assetsTable    table.Model
    projectsTable  table.Model
    assetsPanel    *mnemonic.Button // [1] — focus mnemonic
    projectsPanel  *mnemonic.Button // [2] — focus mnemonic

    // assets-row actions
    editAsset   *mnemonic.Button // e
    deleteAsset *mnemonic.Button // d
    // projects-row actions
    editProject   *mnemonic.Button // e
    selectAssets  *mnemonic.Button // a
    planProject   *mnemonic.Button // p
    deleteProject *mnemonic.Button // d

    // screen-level
    createAsset *mnemonic.Button // c
    register    *mnemonic.Button // r
    back        *mnemonic.Button // b + esc

    set *mnemonic.Set // rebuilt on focus/data transitions

    modal               *modal.Modal
    pendingDeleteAsset  string
    pendingDeleteProj   string

    width, height int
}
```

### Lifecycle

- `newEditProfileScreen(a, profileID)` builds buttons, constructs the
  focus handler with `focus.ModCtrl`, registers each empty table via
  `handler.AddMnemonic(&s.assetsTable, '1')` / `'2'`, seeds both tables
  with empty rows, and calls `rebuildSet()` for the empty-data case
  (only `c`/`r`/`b` + the two `1`/`2` focus mnemonics).
- `Init` returns a `loadCmd` that calls `actions.LoadProfile`. The
  result message (`editProfileLoadedMsg`) carries the loaded
  `*profile.Profile` (plus any `errs.DomainError`).
- On `editProfileLoadedMsg`, populate `prof`, sort assets/projects into
  stable slices, rebuild both tables, call `handler.FocusIndex(0)` so
  the Assets table is the default focus, and rebuild the mnemonic set.

### Update routing (`tea.KeyPressMsg`)

1. If `modal != nil` → forward to modal (matches profiles.go).
2. Call `handler.Update(msg)`; if it reports `handled` → return its cmd.
   This consumes `tab`, `shift+tab`, `ctrl+1`, `ctrl+2`. After a
   focus-changing key, also call `rebuildSet()` so the row-action
   buttons swap to the newly focused table.
3. Call `s.set.Match(msg)` — fires the appropriate button trigger.
4. Otherwise, route the message to the currently-focused table via
   `handler.FocusedComponent()` (type-assert `*table.Model`). On a
   cursor change, rebuild the actions cell on that table only.

### Mnemonic Set (per focus state)

Two combinations are valid; the set is recomputed on focus change
and on data transitions:

- **Assets focused**: `{1, 2, e, d, c, r, b}` — empty assets list drops `e`/`d`.
- **Projects focused**: `{1, 2, e, a, p, d, c, r, b}` — empty projects list drops `e`/`a`/`p`/`d`.

Both combinations are duplicate-free; `mnemonic.Set.Add` panics
otherwise (same load-bearing safety net the Profiles screen uses).

### Row actions

- Assets:
  - `e` → `pushCmd(newEditAssetStub(s.profileID, asset.ID))`.
  - `d` → set `pendingDeleteAsset = asset.ID`, open `NewConfirm("delete-asset", "Are you sure you want to delete asset 'X'?", nil)`.
- Projects:
  - `e` → open `modals.NewEditProject(modals.EditProjectInput{Name, Path, EnabledAgents})` prefilled; on confirm, merge values back into the existing `*project.Manifest` (preserving `ID`, `SelectedAssetIDs`, `CreatedAt`) and call `UpdateProject`.
  - `a` → `pushCmd(newSelectProjectAssetsStub(s.profileID, project.ID))`.
  - `p` → `pushCmd(newPlanProjectStub(s.profileID, project.ID))`.
  - `d` → set `pendingDeleteProj = project.ID`, open `NewConfirm("delete-project", "Are you sure you want to delete project 'X'? (only metadata — repo files are kept)", nil)`. On Yes, call `DeleteProject` (which only removes the `projects/{id}.json` manifest; `project.Delete` does *not* touch the target repo). The prompt itself surfaces the "metadata only" contract so the user is not surprised.

### Screen-level actions

- `c` → `openModal(modals.NewCreateAsset(asset.Manifest{}))`. On confirm, call `CreateAsset(actions.CreateAssetInput{ProfileRef: s.profileID, Manifest: m})`.
- `r` → `openModal(modals.NewRegisterProject(modals.RegisterProjectInput{}))`. The modal extract returns a `*project.Manifest`; on confirm, call `AddProject(actions.AddProjectInput{ProfileRef: s.profileID, Name, Path, EnabledAgents})` (assets selected later in screen 0028).
- `b` (and `esc` via `mnemonic.WithExtraBindingKeys("esc")`) → `popCmd()`.

### Mutations + reload

Reuse `mutationCmd[T]` and `notificationCmd` from `profiles.go`. After
each mutation, the screen reloads the profile so the tables reflect
the new state. Per the same testability rationale as Profiles, route
through a `editProfileMutationDoneMsg{text, severity}` envelope that
`Update` reacts to with `tea.Batch(notificationCmd, loadCmd)`.

### Status bar

`StatusKeys()` returns the row-level mnemonics of the *currently
focused* table, plus `back.Binding()` (the same explicit exception the
Settings + Profiles screens make so the user can see the back hint).
Screen-level `c` / `r` are not duplicated, and the `[1]` / `[2]` focus
buttons are not duplicated either — they are already visible above
each table.

| Focus | StatusKeys |
| ----- | ---------- |
| Assets, non-empty | `e edit`, `d delete`, `b back` |
| Assets, empty | `b back` |
| Projects, non-empty | `e edit`, `a select assets`, `p plan`, `d delete`, `b back` |
| Projects, empty | `b back` |

### Body layout

Heading + button rows + notifications/toast + status bar are fixed
elsewhere; the two tables share the remaining vertical space:

```
[1] Assets
<assets table>                <-- (tableH - 1) / 2 rows

[2] Projects
<projects table>              <-- (tableH - 1) / 2 rows

 [c Create Asset]  [r Register Project]  [b Back]
```

Each `[N]` indicator is the `mnemonic.Button` returned by
`handler.AddMnemonic`, rendered inline before the panel label.
Sizing follows the Profiles pattern: Body receives the shell-reserved
height and clamps each table's height so the total returned by `Body`
matches exactly (regression-guarded by `TestEditProfileScreen_BodyExactlyMatchesRequestedHeight`).

### Stub screens (0027 / 0028 / 0029)

Each stub mirrors `settingsScreen` / `editProfileStub`: stores the
relevant ids, exposes `[Back]` (bound to `b` + `esc`), shows a single
"Coming soon — task 00XX, editing <id>" line, and lists `Back` in
StatusKeys (same exception as Settings, since the body has no other
cue).

| Stub | Ids | Task |
| ---- | --- | ---- |
| `editAssetStub` | `profileID`, `assetID` | 0027 |
| `selectProjectAssetsStub` | `profileID`, `projectID` | 0028 |
| `planProjectStub` | `profileID`, `projectID` | 0029 |

## Tests

Mirroring `profiles_test.go`. Real `*actions.Actions` (wired to a
`t.TempDir`-backed registry), seeded via `app.Service.CreateProfile` +
`InitAsset` + `AddProject` so the loaded profile carries real assets +
projects. A `withProfile` helper bypasses the load command for
UI-state tests.

- `TestEditProfileScreen_InitLoadsProfileFromActions` — Init's cmd resolves to `editProfileLoadedMsg` carrying the seeded profile.
- `TestEditProfileScreen_AssetsFocusedMnemonicSet_PopulatedHasEDB1n2cnrnb_AllUnique` — set labels include `[1]`, `[2]`, `Edit`, `Delete`, `Create Asset`, `Register Project`, `Back`; no panic.
- `TestEditProfileScreen_ProjectsFocusedMnemonicSet_PopulatedHasEnAnPnDB1n2cnrnb_AllUnique` — set labels include `[1]`, `[2]`, `Edit`, `Select Assets`, `Plan`, `Delete`, `Create Asset`, `Register Project`, `Back`; no panic.
- `TestEditProfileScreen_EmptyAssetsListDropsRowMnemonics` — only `1`, `2`, `Create Asset`, `Register Project`, `Back` while focused on an empty Assets table.
- `TestEditProfileScreen_EmptyProjectsListDropsRowMnemonics` — same shape with Projects focused.
- `TestEditProfileScreen_TabCyclesFocus` — assets → projects → assets.
- `TestEditProfileScreen_ShiftTabCyclesBackwards` — projects → assets.
- `TestEditProfileScreen_Ctrl1FocusesAssets` — explicit jump.
- `TestEditProfileScreen_Ctrl2FocusesProjects` — explicit jump.
- `TestEditProfileScreen_BTriggersPop` — `b` and `esc` both emit `PopScreenMsg`.
- `TestEditProfileScreen_AssetsEKeyPushesEditAssetStub` — pushes stub carrying `profileID + assetID`.
- `TestEditProfileScreen_AssetsDKeyOpensDeleteAssetConfirm` — modal id `delete-asset`; `pendingDeleteAsset` set.
- `TestEditProfileScreen_AssetDeleteConfirmedCallsDeleteAsset` — resolving Yes emits `editProfileMutationDoneMsg`; LoadProfile returns the asset gone.
- `TestEditProfileScreen_AssetDeleteRejectedMakesNoServiceCall` — resolving No leaves the asset in place; no cmd.
- `TestEditProfileScreen_ProjectsEKeyOpensEditProjectModal` — modal id `edit-project`; form prefilled (assert via the modal's extract producing the prefilled values).
- `TestEditProfileScreen_ProjectsAKeyPushesSelectProjectAssetsStub`.
- `TestEditProfileScreen_ProjectsPKeyPushesPlanProjectStub`.
- `TestEditProfileScreen_ProjectsDKeyOpensDeleteProjectConfirm`.
- `TestEditProfileScreen_ConfirmedDeleteProjectDoesNotDeleteRepoFiles` — given a project whose `Path` is a real temp dir with a file in it, after confirmed delete the dir + file remain on disk; only the per-profile manifest goes away. Asserts the "metadata only" safety contract from the task description.
- `TestEditProfileScreen_CKeyOpensCreateAssetModal` — modal id `create-asset`.
- `TestEditProfileScreen_RKeyOpensRegisterProjectModal` — modal id `register-project`.
- `TestEditProfileScreen_StatusKeysAssetsFocusedNonEmpty` — `{e, d, b}`.
- `TestEditProfileScreen_StatusKeysProjectsFocusedNonEmpty` — `{e, a, p, d, b}`.
- `TestEditProfileScreen_StatusKeysExcludeScreenLevel` — `c`, `r`, `1`, `2` never appear in StatusKeys.
- `TestEditProfileScreen_StatusKeysFocusedEmpty` — only `b` returned.
- `TestEditProfileScreen_BodyExactlyMatchesRequestedHeight` — table sub-cases (empty, populated, tall, short) the same way Profiles does, since shell relies on the body matching the reserved height.
- `TestEditProfileScreen_LoadErrorEmitsNotification` — corrupt profile manifest, assert the err-branch emits a `NotificationMsg` with `SeverityError`.
- `TestEditProfileScreen_MutationDoneEmitsNotificationAndReload` — same pattern as the Profiles test.

Plus stub tests (mirroring `edit_profile_stub_test.go`):

- `edit_asset_stub_test.go` — Title, StatusKeys=`{b}`, `b` pops, `esc` pops, body mentions asset id.
- `select_project_assets_stub_test.go` — same shape.
- `plan_project_stub_test.go` — same shape.

Plus an update to `profiles_test.go`:

- `TestProfilesScreen_EKeyPushesEditProfileScreen` (renamed from `…Stub`) — asserts the pushed screen is `*editProfileScreen` and carries `profileID == "alpha"`.

## Step-by-step Execution

1. Create the three navigation stubs (`edit_asset_stub.go`, `select_project_assets_stub.go`, `plan_project_stub.go`) + their tests. Build green.
2. Create `edit_profile.go` with the struct + lifecycle + Init/Update/Body/Title/StatusKeys. Use the Profiles screen as the structural template.
3. Wire actions: `LoadProfile`, `CreateAsset`, `DeleteAsset`, `AddProject`, `UpdateProject`, `DeleteProject`. Reuse `mutationCmd[T]` + `notificationCmd` from `profiles.go`.
4. Wire focus handler + the two table widgets, with `[1]` / `[2]` returned by `handler.AddMnemonic`.
5. Wire mnemonic buttons + `rebuildSet` per focus state and per data-empty state.
6. Wire modal flow: `create-asset`, `register-project`, `edit-project`, `delete-asset`, `delete-project` — each with a `handleResolved` branch.
7. Body layout: two-table split + screen-level button row. Get height-exact via test.
8. Swap `newEditProfileStub` → `newEditProfileScreen(s.actions, p.Manifest.ID)` in `profiles.go`. Update the Profiles test that asserted the stub type.
9. Delete `edit_profile_stub.go` + `edit_profile_stub_test.go`.
10. Write the test file. Cover the matrix in the description: `{1, 2, c, r, b, e, d, a, p}` — set builds without panic in both focus states.
11. `make fmt && make build && make test && make lint`.
12. Manual: `./bin/af` → Welcome → Profiles → seed/create a profile → Edit → exercise Tab, ctrl+1/2, e/d on each table, c create asset, r register project, b back. Confirm: confirmed Delete Project does not remove anything on disk under the project's `Path`.
13. Write the changelog at `docs/changelog/2026-06-11_0026-edit-profile-screen.md`.

## Verification

```
make fmt
make build && make test && make lint
./bin/af   # Profiles → Edit Profile → walk every row + screen action
```

Targeted single-package run during iteration:

```
go test ./internal/tui/shell/...
```
