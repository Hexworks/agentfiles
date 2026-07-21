# 0039 — Adopt path selector across all path inputs

Rolled the `pathselector` modal introduced by task 0037 into the three TUI
flows that still collected a filesystem path via a plain `huh.Input`:
Create Profile, Register Profile, and Register Project. Each is now a
two-step flow — the picker opens first (dirs only, no constraint, start
folder defaults to `$HOME`); confirming a folder opens the existing form
with the Path field pre-filled and marked read-only (`Description` ends
in `(read-only)`); Name and (for Register Project) `EnabledAgents` stay
editable. Cancelling the picker aborts the whole flow. When the domain
action fails (ownership conflict, on-disk error), the picker re-opens
seeded at `filepath.Dir(previousPath)` and the follow-on form is
re-seeded with the non-path inputs the user already entered, so the
retry does not force them to re-type Name or re-pick agents.

## Decisions

- `NewSelectPath` signature widened from `NewSelectPath(opts)` to
  `NewSelectPath(id, opts)`. — **Why:** each flow needs a distinct modal
  id so `handleResolved` can dispatch through the usual switch instead of
  tracking hidden state. Task 0037 was the only prior caller and only
  registered the placeholder id `"select-path"`, so the widening is a
  purely additive change to consumers.
- Per-flow typed failure messages (`createProfileFailedMsg`,
  `registerProfileFailedMsg`, `registerProjectFailedMsg`) rather than
  overloading `mutationDoneMsg`. — **Why:** the retry loop needs the
  submitted path to seed the re-opened picker; the shared mutation
  envelope has no place to carry that, and adding one would leak retry
  concerns into every other screen. Success stays on `mutationDoneMsg`
  so the shared refresh path is untouched.
- Stashed non-path inputs live on the screen struct
  (`pendingCreateName`, `pendingRegisterProjectName`,
  `pendingRegisterProjectAgents`), cleared at the start of a new flow
  by the `onCreate` / `onRegisterProject` handlers. — **Why:** avoids
  mutating screen state from inside a `tea.Cmd` closure and keeps the
  clear condition on the *next* flow's open path where it is easy to
  read.
- Read-only path field piggy-backs on `huh.NewInput` with a
  `Description(... + " (read-only)")` marker. — **Why:** `huh` v2 has
  no runtime read-only mode; the marker is the only user-visible
  signal, and it is testable via `form.View()` grep.
- No new ADR. — **Why:** ADR 0021 already recorded the pathselector
  decision for task 0037; "adopt uniformly" is the implicit follow-up
  and does not add a durable trade-off worth its own record.

## Assumptions

- `edit_project.go` is out of scope. — **Why:** the description marks
  editing a project's path as a separate bug — the Path field there
  should be non-editable, not two-step-picker-driven.
- `create_file.go` is out of scope. — **Why:** the Path there is an
  asset-relative slug, not a filesystem path, so the picker does not
  apply.

## Other Notes

- Updated `docs/architecture/05-building-block-view.md` — replaced the
  trailing "wiring is a follow-up" note in the `tui/modals/pathselector`
  paragraph with a sentence stating the three flows now open the picker
  as their first step.
- Existing `TestProfilesScreen_CKeyOpensCreateProfileModal` /
  `RKeyOpensRegisterProfileModal` were renamed to
  `..._PathselectorConfirmOpensForm` / `..._Pathselector` counterparts;
  the modal id on the first-modal assertion moved from
  `"create-profile"` / `"register-profile"` to
  `"create-profile-path"` / `"register-profile-path"`.
- `TestRegisterProfile_RejectsEmptyRequiredField` was removed: the
  Register Profile form now has zero editable fields (Path is the
  only field and it is read-only), so there is no required field left
  to reject.

## Path selector wrapper — `NewSelectPath` accepts an id

```go
// before
func NewSelectPath(opts pathselector.Options) (*modal.Modal, errs.DomainError) {
    // ...
    return modal.New("select-path", &notificationTranslator{...}, modal.WithCaption(caption)), nil
}
```

```go
// after — flow-specific id so handleResolved dispatches uniformly
func NewSelectPath(id string, opts pathselector.Options) (*modal.Modal, errs.DomainError) {
    // ...
    return modal.New(id, &notificationTranslator{...}, modal.WithCaption(caption)), nil
}
```

## Read-only path field helper

```go
// before — only pathInput, always validator-gated
func pathInput(value *string, description string) *huh.Input {
    return huh.NewInput().Key("path").Title("Path").
        Description(description).Value(value).Validate(requiredString)
}
```

```go
// after — new helper for the display-only path field used by the
// three two-step flows; no validator, marker on the description so
// tests can assert the field is intentionally non-editable.
func readOnlyPathInput(value *string, description string) *huh.Input {
    return huh.NewInput().Key("path").Title("Path").
        Description(description + " (read-only)").Value(value)
}
```

## Two-step Create Profile — `onCreate` opens the picker

```go
// before
func (s *profilesScreen) onCreate() tea.Cmd {
    s.openModal(modals.NewCreateProfile(modals.CreateProfileInput{}))
    return s.modal.Init()
}
```

```go
// after — picker first; the form is opened by afterCreatePath
// once the pathselector resolves.
func (s *profilesScreen) onCreate() tea.Cmd {
    s.pendingCreateName = ""
    return s.openCreateProfilePathselectorCmd("")
}
```

## Failure re-opens the picker

```go
// after — Update translates the per-flow failed msg into (notification,
// re-open picker at parent of failing path). The follow-on form sees
// the stashed non-path inputs so the user does not re-type them.
case createProfileFailedMsg:
    return s, tea.Batch(
        notificationCmd(m.severity, m.text),
        s.openCreateProfilePathselectorCmd(filepath.Dir(m.path)),
    )
```
