---
task: 0039
type: feature
---

# Plan — Adopt path selector across all path inputs

Companion to [./description.md](./description.md). Rolls the
`pathselector` modal (task 0037) into every TUI flow that still collects
a filesystem path via `huh.Input`.

## Scope Recap

Three shell entry points, all currently opening a form with a free-form
Path input:

1. **Create Profile** — `profilesScreen.onCreate` → `modals.NewCreateProfile`
   (`internal/tui/shell/profiles.go:305`).
2. **Register Profile** — `profilesScreen.onRegister` → `modals.NewRegisterProfile`
   (`internal/tui/shell/profiles.go:310`).
3. **Register Project** — `editProfileScreen.onRegisterProject` →
   `modals.NewRegisterProject`
   (`internal/tui/shell/edit_profile.go:612`).

Each becomes: **pathselector first → form with Path pre-filled,
read-only**. Cancel on the pathselector aborts. Action failure re-opens
the pathselector seeded with `filepath.Dir(previousPath)`.

## Design Decisions

- **Read-only Path field:** Follow the Edit Asset `typ` field pattern
  (`internal/tui/shell/edit_asset.go:234`) — a `huh.NewInput` with the
  value pre-filled and a `Description` string ending in `(read-only)`.
  Truly disabling focus is out of scope: `huh` has no read-only mode and
  the task test only asserts the description marker.
- **Pathselector ID per flow:** `NewSelectPath` currently hard-codes the
  modal id `"select-path"`. Extend the signature to
  `NewSelectPath(id string, opts pathselector.Options)` so each caller
  registers a distinct id and can route ResolvedMsg through the usual
  `handleResolved` switch instead of tracking hidden state. No other
  call sites exist (task 0037 shipped only the wrapper), so the change
  is purely additive to consumers.
- **Retry-after-error path capture:** Introduce per-flow typed messages
  (`createProfileFailedMsg`, `registerProfileFailedMsg`,
  `registerProjectFailedMsg`) that carry the submitted input alongside
  the error text/severity. The action-runner returns one of these on
  failure and the existing `mutationDoneMsg` on success, so the generic
  `mutationCmd` stays untouched for every other screen. The failed
  message handler emits the standard notification and re-opens the
  pathselector seeded with `filepath.Dir(previousPath)`; the follow-on
  form is re-seeded with the non-path inputs (Name, EnabledAgents) so
  the user does not re-type them.
- **Options:** Dirs only (`ShowFiles: false`), no `ConstraintRoot`,
  `StartFolder: ""` (defaults to `$HOME`). `FollowSymlinks` stays at the
  safe default (`false`). Each flow supplies a distinct caption
  (`"Select profile folder"`, `"Select existing profile folder"`,
  `"Select project root"`).

## Execution Plan

Every step below is a mechanical edit unless flagged otherwise.

### 1 — Shared modal helpers

- Extend `NewSelectPath` in `internal/tui/modals/select_path.go` to
  take an `id string` as the first parameter and pass it through to
  `modal.New`.
- Add `readOnlyPathInput(value *string, description string) *huh.Input`
  in `internal/tui/modals/fields.go`. Same construction as `pathInput`
  minus the validator; concatenate `" (read-only)"` onto the caller's
  description so the marker is testable.
- Do **not** touch `internal/tui/modals/pathselector/*` — out of scope.

### 2 — Modal builders

Update each of the three `buildXxx` helpers so the Path field is the
read-only variant, seeded from the incoming `initial.Path`:

- `internal/tui/modals/create_profile.go`
- `internal/tui/modals/register_profile.go`
- `internal/tui/modals/register_project.go`

`Name` and `EnabledAgents` remain editable. `NewXxx` signatures are
unchanged; the `initial` payload already carries `Path`.

### 3 — Modal tests

Add to the existing `*_test.go` files:

- `TestBuildCreateProfile_PathReadOnly` — assert the `path` field is
  present, its bound value equals the seeded path, and its `Description`
  string contains `"read-only"`.
- Same for `TestBuildRegisterProfile_PathReadOnly` and
  `TestBuildRegisterProject_PathReadOnly`.

Use the existing `buildXxx` factories (they already return the `*huh.Form`
plus the state pointer) and reach into the form via
`form.GetFocusedField()` or by iterating groups — mirror whatever
`create_asset_test.go` does for field introspection.

### 4 — Profiles screen wiring (`internal/tui/shell/profiles.go`)

- Replace `onCreate` / `onRegister` bodies so each opens
  `modals.NewSelectPath("create-profile-path", opts)` /
  `modals.NewSelectPath("register-profile-path", opts)` instead of the
  final form. Handle the wrapper's `errs.DomainError` return by
  surfacing a notification and skipping the modal (defensive — the
  `$HOME` default should never fail).
- Add two new `handleResolved` cases:
  - `"create-profile-path"` → on Confirmed, extract
    `pathselector.ResultFromMsg`, open
    `modals.NewCreateProfile(modals.CreateProfileInput{Path: result.Path})`.
    On Cancelled, do nothing (flow aborted).
  - `"register-profile-path"` → same, opening `NewRegisterProfile`.
- Replace the mutationCmd-only submissions in `afterCreate` /
  `afterRegister` with a closure that returns either
  `mutationDoneMsg{...}` on success or the new
  `createProfileFailedMsg{path, text, severity}` /
  `registerProfileFailedMsg{path, text, severity}` on error.
- Add `case createProfileFailedMsg:` and `case registerProfileFailedMsg:`
  branches in `Update` that emit `notificationCmd(...)` **and** re-open
  the corresponding pathselector via `NewSelectPath` with
  `Options{StartFolder: filepath.Dir(m.path), ShowFiles: false, Caption: ...}`.

### 5 — Edit-profile screen wiring (`internal/tui/shell/edit_profile.go`)

- Add a `modalKindRegisterProjectPath` constant next to the existing
  `modalKindRegisterProject`.
- `onRegisterProject` opens
  `modals.NewSelectPath("register-project-path", opts)` under the new
  kind.
- `handleResolved` picks up the new kind: on Confirmed, open
  `modals.NewRegisterProject(RegisterProjectInput{Path: result.Path})`
  under `modalKindRegisterProject` (unchanged path from there).
- Replace `afterRegisterProject`'s mutationCmd with the same
  success-or-typed-failure pattern. Add
  `case registerProjectFailedMsg:` in `Update` that notifies and
  re-launches the pathselector seeded with
  `filepath.Dir(m.path)` and re-seeds Name + EnabledAgents so those
  survive the retry.

### 6 — Screen tests

Extend the existing screen tests so the new two-step flow is covered:

- `internal/tui/shell/profiles_test.go`
  - `TestProfilesScreen_CKeyOpensCreateProfilePathselector` — press `c`,
    assert `s.modal.ID() == "create-profile-path"`.
  - `TestProfilesScreen_CreateProfilePathselectorConfirmOpensForm` —
    dispatch a Confirmed `ResolvedMsg{ID: "create-profile-path",
    Value: pathselector.Result{Path: "/tmp/x", IsDir: true}}`, assert
    the next modal is `create-profile` and its initial state has
    `Path == "/tmp/x"`.
  - `TestProfilesScreen_CreateProfilePathselectorCancelClearsModal` —
    dispatch Cancelled ResolvedMsg, assert no follow-on modal opens.
  - `TestProfilesScreen_CreateProfileFailureReopensPathselector` — feed
    the form ResolvedMsg through, drain the failure cmd, feed
    `createProfileFailedMsg{path: "/tmp/x", ...}` through Update, assert
    the modal is again `create-profile-path` (options `StartFolder ==
    "/tmp"` cannot be introspected from outside; skip that assertion or
    expose a testing helper that returns the last requested options).
  - Same trio for Register Profile.
- `internal/tui/shell/edit_profile_test.go` gets equivalent tests for
  the Register Project two-step flow.

Existing `TestProfilesScreen_*OpensXxxModal` tests still hold but must
be renamed to reflect that `c`/`r` now open the pathselector, not the
form.

### 7 — Documentation

- Update the pathselector paragraph in
  `docs/architecture/05-building-block-view.md:261` — the trailing note
  "This task adds the modal component only; wiring it into existing
  screens (register project, register asset, edit profile, …) is a
  follow-up." should be replaced with a sentence stating that
  Create Profile / Register Profile / Register Project now open the
  pathselector as their first step.
- Add a changelog entry under `docs/changelog/` describing the flow
  change and the retry-after-error behavior.
- Glossary (`docs/glossary.md`): no new terms; skip.
- No new ADR — the decision to use pathselector was made in ADR that
  accompanied task 0037. If reviewers want an "adopt uniformly" ADR,
  add one under `docs/adr/`; my recommendation is to skip (the rule is
  already implicit).

## Files Touched

| File | Change |
| --- | --- |
| `internal/tui/modals/select_path.go` | `NewSelectPath(id, opts)` signature |
| `internal/tui/modals/fields.go` | new `readOnlyPathInput` helper |
| `internal/tui/modals/create_profile.go` | use read-only path input |
| `internal/tui/modals/register_profile.go` | use read-only path input |
| `internal/tui/modals/register_project.go` | use read-only path input |
| `internal/tui/modals/create_profile_test.go` | assert read-only marker |
| `internal/tui/modals/register_profile_test.go` | assert read-only marker |
| `internal/tui/modals/register_project_test.go` | assert read-only marker |
| `internal/tui/shell/profiles.go` | two-step flows + failed msgs |
| `internal/tui/shell/profiles_test.go` | new flow tests |
| `internal/tui/shell/edit_profile.go` | two-step flow + failed msg + new modalKind |
| `internal/tui/shell/edit_profile_test.go` | new flow test |
| `docs/architecture/05-building-block-view.md` | pathselector paragraph |
| `docs/changelog/…` | new entry |

## Quality Gate

Per repo convention:

```bash
make build
make test
make lint
```

Plus targeted:

```bash
go test ./internal/tui/modals -run 'TestBuildCreateProfile|TestBuildRegisterProfile|TestBuildRegisterProject'
go test ./internal/tui/shell -run 'CreateProfilePath|RegisterProfilePath|RegisterProjectPath'
```

Manual smoke (per description Verification): `./bin/af` → Profiles →
Create Profile → confirm folder → confirm form → profile appears.
Repeat for Register Profile and Register Project.

## Risks

- **Modal ID change ripple:** any TUI test that hard-codes
  `"create-profile"` / `"register-profile"` on the first-modal
  assertion must move that check to the second modal step. Ran the
  grep in the audit; the count is small.
- **Failure-msg envelope drift:** adding three per-flow msgs is more
  surface than reusing `mutationDoneMsg`. The trade is worth it — the
  generic envelope has no place to carry the submitted path, and
  overloading it would leak retry concerns into every other screen.
- **`$HOME` unset:** `pathselector.Options` already defaults to `/`
  when `HOME` is empty, so the flow still opens.
