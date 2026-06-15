# Plan — Task 0025: Profiles Screen

Cross-links:
- Task description: [./description.md](./description.md)
- Parent task (UI refactor): [../../done/0015_task_refactor_ui/description.md](../../done/0015_task_refactor_ui/description.md)
- ADR 0011 (screen router): [../../../docs/adr/0011-tui-screen-router.md](../../../docs/adr/0011-tui-screen-router.md)

## Context

The TUI shell (task 0021), system + form modals (0022, 0023), mnemonic components (0020), Actions + Notifications (0019), and the Welcome / Settings screens (0024) are landed. Welcome's "Profiles" entry currently pushes `profilesStub`, a placeholder. This task replaces that stub with the real Profiles screen — the first entity-management surface in the TUI — and adds a minimal Edit Profile stub so the row-level `e` action has a target until task 0026 lands the real Edit Profile screen.

Behavior matches the parent task 0015 spec: bubbles/table listing profiles, row-level `[Edit] / [Delete]` mnemonic buttons on the cursor row, screen-level `[Create New Profile] / [Register Profile]` buttons below the table, a two-step delete confirmation for folder removal, and a notification per mutation. Status bar exposes only row-level mnemonics (`e`, `d`) per the parent's screen-level-buttons-not-duplicated rule.

## Files To Add

- `internal/tui/shell/profiles.go` — the real `profilesScreen` Screen.
- `internal/tui/shell/profiles_test.go` — behavior tests.
- `internal/tui/shell/edit_profile_stub.go` — `editProfileStub` (a placeholder that captures the profile id; replaced in task 0026).
- `internal/tui/shell/edit_profile_stub_test.go` — minimal tests.

## Files To Modify

- `internal/tui/shell/welcome.go` — `newWelcomeScreen` takes `*actions.Actions`; Profiles row pushes `newProfilesScreen(a)` instead of the stub.
- `internal/tui/shell/welcome_test.go` — update the wiring (use `actions.New(app.New(t.TempDir() + "/registry.json"))`), rename `TestWelcomeScreen_EnterOnProfilesPushesProfilesStub` to `…PushesProfilesScreen`, assert pushed type is `*profilesScreen`.
- `internal/tui/shell/shell.go` — forward `m.actions` to `newWelcomeScreen`.

## Files To Delete

- `internal/tui/shell/profiles_stub.go`
- `internal/tui/shell/profiles_stub_test.go`

## Reused Existing Components

| Need                            | Reuse                                                                                                                        |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Screen contract / push / pop    | `internal/tui/shell/screen.go` (`Screen`, `PushScreenMsg`, `PopScreenMsg`, `pushCmd`, `popCmd`)                               |
| Load profiles                   | `actions.Actions.LoadProfiles() ([]*profile.Profile, errs.DomainError)` (`internal/actions/profiles.go`)                     |
| Create / Register / Delete      | `actions.CreateProfile`, `RegisterProfile`, `DeleteProfile`, `DeleteProfileWithFolder` + their `*Input` structs (same file)  |
| Notification bridge             | `notifications.From[T]` (`internal/tui/notifications/bridge.go`) — used for all mutation actions                              |
| Create Profile modal            | `modals.NewCreateProfile(initial modals.CreateProfileInput) *modal.Modal` (`internal/tui/modals/create_profile.go`)           |
| Register Profile modal          | `modals.NewRegisterProfile(initial modals.RegisterProfileInput) *modal.Modal` (`internal/tui/modals/register_profile.go`)     |
| Confirmation modal              | `modal.NewConfirm(id, prompt, opts)` (`internal/tui/components/modal/confirm.go`); read `ResolvedMsg.Confirmed`               |
| Mnemonic buttons + uniqueness   | `mnemonic.New`, `mnemonic.NewSet`, `Set.Add` (panics on duplicate), `Set.Match`, `Set.View` (`internal/tui/components/mnemonic/`) |
| Table widget                    | `charm.land/bubbles/v2/table` (same import as `notificationsmodal`)                                                            |
| Modal-over-body compositing     | `*modal.Modal.Render(background, w, h)` — same idiom as `notificationsScreen` but kept local to the screen instead of as a sub-screen |
| Modal sizing floor              | `modalSize(width, height)` from `screen.go` (already in package)                                                              |

## Screen Design

### Type

```go
type profilesScreen struct {
    actions  *actions.Actions
    profiles []*profile.Profile
    table    table.Model
    loadErr  errs.DomainError

    // mutually-exclusive overlay state
    modal *modal.Modal
    // tracks which two-step delete is in flight
    pendingDeleteID string

    // mnemonic buttons (rebuilt on data refresh)
    edit, delete                *mnemonic.Button
    create, register            *mnemonic.Button
    // set holds whatever buttons are currently visible — refreshed on
    // selection-state changes so duplicate-mnemonic checks run at Add time.
    set *mnemonic.Set

    width, height int
}
```

### Lifecycle

- `Init()` → returns a `tea.Cmd` that calls `LoadProfiles` and emits `profilesLoadedMsg{profiles, err}`. Action call sits in a `tea.Cmd` to satisfy "no slow I/O in Update" (`tui.md`).
- `Update(profilesLoadedMsg)` → store profiles, rebuild table rows + mnemonic set. If `err != nil` also emit `NotificationMsg` with the domain severity.
- `Update(WindowSizeMsg)` → store dimensions, recompute table height (= body height − button-row height − 1 spacer), feed to `table.SetHeight` / `SetWidth`.
- `Update(KeyPressMsg)` → if modal is open, forward to modal; otherwise consult `set.Match(kp)`; otherwise forward to `table.Update`.
- `Update(modal.ResolvedMsg)` → branch on `msg.ID` and `Confirmed`:
  - `"create-profile"` + Confirmed → run `notifications.From(...CreateProfile, "Profile created")` then refresh via `Init`'s load cmd; clear modal.
  - `"register-profile"` + Confirmed → same with `RegisterProfile`.
  - `"delete-profile-1"` + Confirmed → open `"delete-profile-2"`.
  - `"delete-profile-1"` + !Confirmed → clear modal, clear `pendingDeleteID`.
  - `"delete-profile-2"` + Confirmed → `DeleteProfileWithFolder`, then refresh.
  - `"delete-profile-2"` + !Confirmed → `DeleteProfile` (keep folders), then refresh.
- `Body(w, h)` → renders title-less body (shell owns the title): table + spacer + button row (the screen-level `[Create New Profile] [Register Profile]` joined by `mnemonic.Set.View`-style spacing). When `modal != nil`, returns `modal.Render(body, w, h)` so the dialog composites centered.
- `Title()` → `"Profiles"`.
- `StatusKeys()` → row-level bindings only: `[]key.Binding{s.edit.Binding(), s.delete.Binding()}` when a row exists; `nil` otherwise. Screen-level `c`/`r` are visible on the button row and must not be duplicated (per parent task 0015 rule and `renderStatusBar` doc).

### Mnemonic Set

Rebuilt every time the selection state changes (i.e. after `profilesLoadedMsg` and after each row-change). The set is what catches duplicate mnemonics at `Set.Add` time — the load-bearing safety check per task description.

- No profiles: `set` holds `create ('c')`, `register ('r')`.
- Profiles present, row selected: `set` holds `edit ('e')`, `delete ('d')`, `create ('c')`, `register ('r')`.

Mnemonic labels per the task mockup: `[Edit]`, `[Delete]`, `[Create New Profile]`, `[Register Profile]`.

### Two-Step Delete

State machine inside the screen, not a sub-screen — confirmation modals are hosted in-place via `modal *modal.Modal`. On `d`:

1. Capture the cursor row's profile id into `pendingDeleteID`.
2. Open `modal.NewConfirm("delete-profile-1", `Are you sure you want to delete profile "{name}"?`, …)`.
3. `ResolvedMsg{ID: "delete-profile-1", Confirmed: false}` → discard, clear `pendingDeleteID`, no service call.
4. `ResolvedMsg{ID: "delete-profile-1", Confirmed: true}` → open `modal.NewConfirm("delete-profile-2", "Also delete profile folder on disk?", …)`.
5. `ResolvedMsg{ID: "delete-profile-2", Confirmed: false}` → `DeleteProfile{ProfileRef: pendingDeleteID}` (keep folders).
6. `ResolvedMsg{ID: "delete-profile-2", Confirmed: true}` → `DeleteProfileWithFolder{ProfileRef: pendingDeleteID}`.

All mutation calls wrap through `notifications.From` so the outcome surfaces via the toast + log without the screen needing to thread severity.

### Edit Profile Stub

`editProfileStub` is a tiny placeholder that satisfies the Screen interface:

- Field `profileID string` (carried so 0026 can use it).
- Field `back *mnemonic.Button` with binding `b` and extra key `esc` (same pattern as `settingsScreen`).
- `Title() → "Edit Profile"`.
- `Body()` → `"Editing profile {{id}} — task 0026"` plus the back button right-aligned (mirror `settingsScreen.Body`).
- `StatusKeys()` → `[]key.Binding{back.Binding()}` (Back stays in the bar — same Settings-style exception).
- `Update()` → `b` / `esc` → `popCmd()`.

## Test Plan (`profiles_test.go`)

Uses real `actions.Actions` over a `t.TempDir()` registry, same pattern as `shell_test.go::newTestShell`. Some tests use a `profilesScreen` constructed directly with hand-seeded profiles state to avoid waiting for the load cmd; the wiring test pumps `Init`'s cmd through.

| Test                                                          | Behavior                                                                                   |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `TestProfilesScreen_InitLoadsProfilesFromActions`             | `Init().()` returns `profilesLoadedMsg` carrying the seeded profiles                       |
| `TestProfilesScreen_NoProfiles_MnemonicSetHasOnlyCR`          | Empty registry → set has `c`, `r`; no `e`, `d`                                             |
| `TestProfilesScreen_WithProfiles_MnemonicSetHasECDR_AllUnique` | One profile + cursor on it → set has `e`, `d`, `c`, `r`; `Set.Add` never panics            |
| `TestProfilesScreen_DuplicateMnemonicWouldPanic`              | Sanity: registering two buttons with the same rune through `Set.Add` panics (asserts the safety net is hooked up by re-running construction with bad input) |
| `TestProfilesScreen_StatusKeysExcludeScreenLevelButtons`      | Returned bindings carry only `e edit`, `d delete` — never `c create` or `r register`        |
| `TestProfilesScreen_StatusKeysEmptyWhenNoProfiles`            | Empty state → `StatusKeys()` returns nil                                                   |
| `TestProfilesScreen_CKeyOpensCreateProfileModal`              | After `c`, `s.modal != nil`, `s.modal.ID() == "create-profile"`                            |
| `TestProfilesScreen_RKeyOpensRegisterProfileModal`            | After `r`, `s.modal.ID() == "register-profile"`                                            |
| `TestProfilesScreen_EKeyPushesEditProfileStub`                | After `e`, cmd resolves to `PushScreenMsg{Screen: *editProfileStub}` with the row's id     |
| `TestProfilesScreen_DKeyOpensFirstConfirmModal`               | After `d`, `s.modal.ID() == "delete-profile-1"`, `pendingDeleteID == row.id`               |
| `TestProfilesScreen_DeleteStep1NoMakesNoServiceCall`          | `ResolvedMsg{ID:"delete-profile-1", Confirmed:false}` → modal cleared, no second modal, profile still in registry |
| `TestProfilesScreen_DeleteStep2NoCallsKeepFolders`            | Yes/No → registry entry gone, on-disk folder still exists (assert via Stat)                |
| `TestProfilesScreen_DeleteStep2YesCallsDeleteFolders`         | Yes/Yes → registry entry gone, on-disk folder removed                                      |
| `TestProfilesScreen_MutationRefreshesProfiles`                | After a Create resolves successfully, an additional load-cmd runs and profiles list grows  |
| `TestProfilesScreen_LoadErrorEmitsNotification`               | Seeding a corrupt manifest → `Update(profilesLoadedMsg{err})` returns a `NotificationMsg` cmd carrying the error severity |
| `TestProfilesScreen_TitleAndBodyContainRequiredText`          | `Title() == "Profiles"`, body contains `[Create New Profile]`, `[Register Profile]`        |

Edit Profile stub tests (`edit_profile_stub_test.go`):

| Test                                                  | Behavior                                                              |
| ----------------------------------------------------- | --------------------------------------------------------------------- |
| `TestEditProfileStub_BTriggersPop`                    | `b` → `PopScreenMsg`                                                  |
| `TestEditProfileStub_EscTriggersPop`                  | `esc` → `PopScreenMsg`                                                |
| `TestEditProfileStub_TitleAndStatusKeysExposeBack`    | Title is `"Edit Profile"`, status keys carry `b Back`                  |
| `TestEditProfileStub_BodyIncludesProfileID`           | Body includes the captured `profileID`                                |

## Step-By-Step Execution

1. Update `tasks/current/0025_feature_profiles-screen/description.md` frontmatter `status: pending → in-progress`; add the `## Plan` link section pointing at `./plan.md`.
2. Create branch `feature/profiles-screen`.
3. Write `internal/tui/shell/edit_profile_stub.go` (small; clone `settings.go` shape).
4. Write `internal/tui/shell/edit_profile_stub_test.go`.
5. Write `internal/tui/shell/profiles.go` — type, constructor, mnemonic-set builder, `Init` load cmd + `profilesLoadedMsg`, `Update` modal/key/table dispatch, `Body` with bubbles/table + button row + modal composite, `Title`, `StatusKeys`.
6. Write `internal/tui/shell/profiles_test.go` — start with empty/state tests (no I/O), then wire integration cases using real `actions.Actions` + `t.TempDir()`.
7. Update `internal/tui/shell/welcome.go`: `newWelcomeScreen(globals, a)` signature; Profiles row pushes `newProfilesScreen(a)`.
8. Update `internal/tui/shell/shell.go`: pass `actions` to `newWelcomeScreen`.
9. Update `internal/tui/shell/welcome_test.go`: thread the actions handle through; rename + retype the Profiles-push assertion.
10. Delete `profiles_stub.go` and `profiles_stub_test.go`.
11. Run `make fmt && make build && make test && make lint`.
12. Manual smoke pass via `./bin/af`: Welcome → Profiles → Create → Register → Edit (lands on stub) → Delete (both branches) → notification toast appears for each.
13. Edit frontmatter `status: in-progress → in-review`.
14. Write `docs/changelog/2026-06-11_0025-profiles-screen.md` from the changelog template.
15. Commit (no AI footer per repo CLAUDE.md).

## Architecture & Docs Touches

- No ADR required: screen router pattern is unchanged (ADR 0011 still covers it).
- No new guideline: behavior matches existing `tui.md` patterns (Init-loaded data, mnemonics for sticky shortcuts, modals composited over body).
- No `docs/architecture/` edits: the building-block view (05) does not enumerate individual screens.

## Verification

```bash
make fmt
make build
make test
make lint
./bin/af   # Welcome → Profiles → Create / Register / Edit / Delete flows
```

Expected:
- All tests pass (including the new `profiles_test.go` + `edit_profile_stub_test.go` and updated `welcome_test.go`).
- `./bin/af`: Profiles screen lists existing profiles in a bordered table; cursor row shows `[Edit] [Delete]`; pressing `c` opens the Create Profile form; on submit, a toast confirms; profile appears in the table without restart. `d` walks the two-step confirmation: first dialog gates the delete itself, second gates on-disk folder removal. `e` pushes the stub; `b` or `esc` returns.
