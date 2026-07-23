---
id: 0040
type: bug
status: done
topics: tui, charm
depends_on: 0039, 0037
notes: |
    I've noticed that the solution for #0039 is buggy. When I select a path
    it goes back to the profile registration form (i've only tried this on
    the register profile action). The path form element says it is
    read-only, but i can still edit it, and pressing <enter> opens the path
    selector again as opposed to submitting the form.
---

# Path selector re-opens on Enter in profile form

## Context

Task 0039 introduced a two-step flow for the three "existing-folder"
modals (`register-profile`, `create-profile`, `register-project`): the
pathselector runs first, then a `huh.Form` opens with the picked path
already filled in and the user completes the remaining fields.

The path field in the follow-on form was meant to be read-only. The
current helper `internal/tui/modals/fields.go:readOnlyPathInput` builds
a plain `huh.NewInput` and only appends `" (read-only)"` to the
description — huh's `Input` has no runtime read-only mode, so the field
stays fully editable and keeps consuming keypresses. Symptoms observed
in Register Profile:

- The "(read-only)" marker in the description is misleading — typing
  edits the value.
- With a single-field group, pressing Enter on the focused input does
  not complete the form. The keypress is not swallowed by the form and
  ends up re-triggering the same code path that opened the picker in
  step 1, so the picker reappears instead of the form submitting.

huh already ships a display-only field (`huh.Note`, `field_note.go`)
that renders text but rejects rune input and advances on Enter. The
edit_asset screen's read-only "Type" field achieves the same effect a
different way (its `focus.Handler` deliberately skips the field, see
`internal/tui/shell/edit_asset.go:113-116`), but that pattern requires
a custom focus manager and does not transplant into the plain
`huh.Form`s used by the modals here. Swapping the helper to return a
`huh.Note` is the smallest change that fixes both symptoms in every
call site.

## Acceptance Criteria

- [ ] `internal/tui/modals/fields.go` exposes `pathDisplayNote(value
    *string, description string) *huh.Note` in place of
      `readOnlyPathInput`; no `readOnlyPathInput` references remain in
      the package.
- [ ] `internal/tui/modals/create_profile.go`,
      `internal/tui/modals/register_profile.go`, and
      `internal/tui/modals/register_project.go` all build their path
      row with `pathDisplayNote` and pass the picked path in as the
      Note value.
- [ ] For each of the three forms, a unit test walks the built form
      and asserts the path row is `*huh.Note`, not `*huh.Input`.
- [ ] For each of the three forms, a unit test feeds a rune keypress
      (`tea.KeyPressMsg` for `x`) to the form and asserts the backing
      `state.Path` is unchanged.
- [ ] For register-profile and register-project (single-Note groups),
      a unit test feeds `Enter` and asserts `form.State ==
    huh.StateCompleted` — no picker re-open, no key bleed-through.
- [ ] For create-profile (Name + Note), a unit test seeds a valid
      Name, advances focus off the Name field, feeds `Enter` on the
      Note row, and asserts `form.State == huh.StateCompleted`.
- [ ] Manual repro path clears: `./bin/af` → Profiles → `r` → pick a
      folder → Enter → register-profile form appears → typing any
      rune leaves the path field unchanged → Enter completes the
      form and the profiles table shows the "Profile registered"
      notification.

## Out of scope

- The read-only Type field on the Edit Asset screen. It already
  behaves correctly via `focus.Handler` and is not part of the
  two-step modal pattern.
- Adding a `.Next(true).NextLabel(...)` explicit button to the Note.
  Plain Note + Enter is enough; explicit button is left as a possible
  follow-up if the affordance is judged unclear later.
- Any pathselector modal internals — the bug is entirely in the
  follow-on form, not in the picker.

## Plan

[plan.md](./plan.md)

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/tui/modals/...` — updated tests exercise the
  Note swap plus the rune-ignored and Enter-completes assertions for
  all three forms.
- Manual smoke — Register Profile: `./bin/af → Profiles → r → select a
  folder → Enter → register-profile form → press 'x' (path field
  unchanged) → Enter → "Profile registered" notification, back on the
  profiles table, picker does NOT re-open.
- Manual smoke — Create Profile: `./bin/af → Profiles → c → select
  folder → Enter → create-profile form with Name focused → type a
  fresh name → tab to path row → press 'x' (path unchanged) → Enter →
  "Profile … created" notification.
- Manual smoke — Register Project: `./bin/af → open an existing profile
  → Projects → r → select a folder → Enter → register-project form →
  press 'x' (path unchanged) → Enter → "Project registered"
  notification.
