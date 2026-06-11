# 0026 changes

Replaced the `editProfileStub` placeholder with the real Edit Profile
screen — the second entity-management surface in the TUI. The screen owns
two `bubbles/table` views (Assets and Projects), a `focus.Handler`
configured with `WithModifier(focus.ModCtrl)` that exposes the visible
`[1]` / `[2]` ctrl-digit mnemonics, row-level action buttons that swap
with the focused table, and screen-level `[Create Asset]` /
`[Register Project]` / `[Back]` mnemonic buttons. Confirmation +
form modals composite over the screen body instead of pushing
sub-screens. Every mutation flows back through a custom done envelope
(same pattern as the Profiles screen) so the user sees a toast + log
entry and the tables reload from the registry-backed profile.

Three navigation stubs ship alongside (`editAssetStub`,
`selectProjectAssetsStub`, `planProjectStub`) so the row-level `e`/`a`/`p`
actions have valid push targets until tasks 0027/0028/0029 land the
real screens. Each mirrors `editProfileStub`'s shape (`[Back]`-only
keymap, captured ids, title + Coming Soon body).

## Decisions

- **Tables stored as `*table.Model`, registered with the focus handler.** **Why:** The focus handler tracks raw component references; mutating Update returns a value, so the screen needs a stable pointer it can dereference inside `routeToFocusedTable`. Storing pointers also matches the `focus/main.go` example so future maintainers find the same shape across screens.
- **`rebuildSet()` is called on every focus change and every selection-state transition.** **Why:** `mnemonic.Set.Add` panics on duplicates. Re-running the build after `handler.Update` reports `handled=true` turns the safety net into a continuous invariant — the load-bearing piece the description's "candidate alphabet `{1, 2, c, r, b, e, d, a, p}`" test asks us to enforce. With Assets focused the set is `{1, 2, e, d, c, r, b}`; with Projects focused it becomes `{1, 2, e, a, p, d, c, r, b}`. Both are disjoint by construction.
- **Status bar carries `b back` on top of the focused table's row mnemonics.** **Why:** The parent task 0015 status-bar rule says screen-level labelled buttons must not be repeated, *and* the task 0026 description explicitly carves out the same `b back` exception the Settings / Profiles screens already use. Without `b` the status bar offers no visible cue for navigating away.
- **`[1]` / `[2]` focus mnemonics are added to the mnemonic set but excluded from `StatusKeys`.** **Why:** Adding them to the set covers the description's uniqueness matrix without a separate ad-hoc check; excluding them from the status bar keeps the bar visually clean — the panel-title `[1]` / `[2]` indicators are already next to each table.
- **Delete Project prompt explicitly says "only metadata — repo files are kept".** **Why:** `app.Service.DeleteProject` only removes the per-profile `projects/{id}.json` manifest; the target repository is untouched. Surfacing that contract in the dialog text matches the "ask the user before destructive actions" guideline and prevents the most likely misunderstanding the screen could cause.
- **Mutation flow envelope (`editProfileMutationDoneMsg`) mirrors the Profiles screen.** **Why:** `tea.Sequence` wraps children in an unexported message type that tests cannot inspect. A custom envelope runs the action synchronously inside the Cmd, then lets `Update` emit `tea.Batch(notification, reload)` — ordered by virtue of the action having already completed before the message is dispatched.

## Assumptions

- **Empty data drops the row mnemonics but `[1]` / `[2]` stay.** **Why:** Tab cycling and ctrl+1 / ctrl+2 must remain available so the user can move focus to a non-empty table; the row buttons (`e`, `d`, `a`, `p`) are useless when there is nothing under the cursor and `mnemonic.Set` panics if they collide with a future button, so they drop out cleanly.
- **Default focus on profile load is the Assets table.** **Why:** The description doesn't pin a default; assets are usually richer than the initial empty projects list and matches the visual top-down order of the panels.
- **Edit Project preserves `ID`, `SelectedAssetIDs`, `CreatedAt`.** **Why:** `EditProjectInput` exposes only `Name`, `Path`, `EnabledAgents` — the modal documents the contract: "the screen … is responsible for merging these values back in while preserving `ID`, `SelectedAssetIDs`, and `CreatedAt`." Merge happens in `afterEditProject` by mutating the already-loaded `*project.Manifest` in place.

## Other Notes

- No ADR added: ADR 0011 (TUI Screen Router) already covers this surface; no new architectural decision is introduced.
- No new `docs/guidelines/` entries: behavior matches the existing `tui.md` patterns (Init-loaded data, mnemonics for sticky shortcuts, modals composited over body, notifications via `NotificationMsg`).
- Three stub screens (`edit_asset_stub.go`, `select_project_assets_stub.go`, `plan_project_stub.go`) added with `[Back]`-only keymaps so the Edit Profile row actions for tasks 0027/0028/0029 have valid push targets. They will be replaced by the real screens in their respective tasks.

## Edit Profile screen Screen implementation

A new `editProfileScreen` Screen that owns two `*table.Model` widgets,
ten mnemonic buttons (2 focus panels, 6 row actions, 3 screen-level),
a `focus.Handler` configured with `WithModifier(focus.ModCtrl)`, an
active-modal field, and two pending-delete ids. `Init` returns a
`LoadProfile` command; `Update` first forwards modal traffic, then
lets the focus handler consume `tab` / `shift+tab` / `ctrl+1` / `ctrl+2`,
then matches the mnemonic set, then routes the leftover keypress to
whichever table the handler currently considers focused.

```go
// before — placeholder push from profiles.go
return pushCmd(newEditProfileStub(p.Manifest.ID))
```

```go
// after — the real screen is constructed with the actions handle so it
// can load and mutate per-profile state. The Profiles row only knows
// about the screen constructor; nothing about the row needed to change
// beyond the type.
return pushCmd(newEditProfileScreen(s.actions, p.Manifest.ID))
```

## Two-table focus + mnemonic uniqueness

The screen creates a `focus.Handler` with `ModCtrl` and registers both
tables. `handler.AddMnemonic` returns `*mnemonic.Button` whose `View()`
renders `[1]` / `[2]` with the digit highlighted. Both buttons are
added to the screen's `mnemonic.Set` so its registration-time
uniqueness check covers them too.

```go
// after — focus + mnemonic wiring
s.handler = focus.New(focus.WithModifier(focus.ModCtrl))
s.assetsPanel = s.handler.AddMnemonic(s.assetsTable, '1')
s.projectsPanel = s.handler.AddMnemonic(s.projectsTable, '2')
s.rebuildSet()
```

```go
// after — rebuildSet swaps row buttons based on focus + data state
func (s *editProfileScreen) rebuildSet() {
    set := mnemonic.NewSet()
    set.Add(s.assetsPanel)
    set.Add(s.projectsPanel)
    switch s.handler.Focused() {
    case 0:
        if len(s.assetsList) > 0 {
            set.Add(s.editAsset); set.Add(s.deleteAsset)
        }
    case 1:
        if len(s.projectsList) > 0 {
            set.Add(s.editProject); set.Add(s.selectAssets)
            set.Add(s.planProject); set.Add(s.deleteProject)
        }
    }
    set.Add(s.createAsset); set.Add(s.register); set.Add(s.back)
    s.set = set
}
```

## Delete Project preserves repo files

`app.Service.DeleteProject` only removes the per-profile project
manifest. The screen's confirmation prompt makes that contract
explicit, and `TestEditProfileScreen_ConfirmedDeleteProjectDoesNotDeleteRepoFiles`
verifies a sentinel file inside the project's `Path` survives a
confirmed delete.

```go
// after — confirmation prompt names the "metadata only" contract so
// the user is never surprised
prompt := fmt.Sprintf(
    "Are you sure you want to delete project %q? (only metadata — repo files are kept)",
    p.Name,
)
s.openModal(modal.NewConfirm("delete-project", prompt, nil))
```

## Nav stubs for downstream tasks

Three new stub screens ship so the row actions targeted at upcoming
tasks have valid push targets. Each is the same shape as the original
`editProfileStub` (now removed): `[Back]` mnemonic bound to both `b`
and `esc`, a one-line body that names the captured ids, and a
StatusKeys exception that includes `[Back]` because the body has no
other cue.

```go
// after — captures the ids so the future screen can swap the body
// without changing the call site.
type editAssetStub struct {
    profileID string
    assetID   string
    back      *mnemonic.Button
}
```
