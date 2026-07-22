---
task: 0040
type: bug
status: draft
---

# Plan — Path selector re-opens on Enter in profile form

Related: [description.md](./description.md), task 0039 changelog
`docs/changelog/2026-07-21_0039-adopt-path-selector-across-inputs.md`,
building-block section for `tui/modals/pathselector` in
`docs/architecture/05-building-block-view.md`.

## Root cause recap

`internal/tui/modals/fields.go:readOnlyPathInput` returns a plain
`huh.Input` with `" (read-only)"` glued onto its `Description`. `huh` has
no runtime read-only mode: the field keeps consuming runes and, in a
single-field group, its `Enter` does not advance the form to
`StateCompleted`. The keypress bubbles out of the form, the shell dispatches
it as a "select-path" trigger again, and the picker re-opens.

`huh.NewNote` is the built-in display-only field. `Note.Update` on a
`tea.KeyPressMsg` returns `NextField` unconditionally, `Note.Skip()`
returns `false` when the note is the sole field in its group (see
`field_note.go:WithPosition`), and `GetKey()` returns `""` — no value can
be mutated even if a rune reaches it. Swap the helper and the two
symptoms disappear together.

## Files touched

| File                                              | Change                                                                                                                                     |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `internal/tui/modals/fields.go`                   | Replace `readOnlyPathInput` with `pathDisplayNote(value *string, description string) *huh.Note`. No `readOnlyPathInput` references remain. |
| `internal/tui/modals/register_profile.go`         | Build path row via `pathDisplayNote(&state.Path, ...)`. Extend `buildRegisterProfile` return with `fields []huh.Field` for tests.          |
| `internal/tui/modals/create_profile.go`           | Same swap. Extend `buildCreateProfile` return with `fields []huh.Field`.                                                                   |
| `internal/tui/modals/register_project.go`         | Same swap. Extend `buildRegisterProject` return with `fields []huh.Field`.                                                                 |
| `internal/tui/modals/register_profile_test.go`    | Replace `TestBuildRegisterProfile_PathReadOnly` with the three checks below; update call-sites to new build return.                        |
| `internal/tui/modals/create_profile_test.go`      | Same; keep the Name-required test intact.                                                                                                  |
| `internal/tui/modals/register_project_test.go`    | Same; keep the required-agents tests intact.                                                                                               |
| `docs/architecture/05-building-block-view.md`     | Retarget the `tui/modals/pathselector` paragraph: the follow-on form uses a display-only `huh.Note`, not a read-only Input.                |
| `docs/changelog/2026-07-22_0040-path-selector-reopens-on-enter-in-profile-form.md` | New changelog entry describing the swap and its consequences.                                              |

No new ADRs. No new/updated `docs/guidelines/` files. Glossary unaffected
(no new domain terms).

## `pathDisplayNote` shape

```go
// pathDisplayNote renders the pre-picked path from step 1 as a
// non-editable Note in the follow-on form. huh.Note has no value binding
// so nothing can mutate the path; it also returns NextField on any key
// press, so Enter (or any rune) advances the group without letting the
// keystroke leak back to the shell.
func pathDisplayNote(value *string, description string) *huh.Note {
    return huh.NewNote().
        Title("Path").
        Description(description + "\n" + *value)
}
```

`*string` signature mirrors `nameInput` / `pathInput` for call-site
uniformity. The value is read once at construction — Notes have no
runtime binding.

## Build helper signature (per modal)

```go
func buildRegisterProfile(initial RegisterProfileInput) (
    *huh.Form,
    *RegisterProfileInput,
    func(*huh.Form) any,
    []huh.Field,
) {
    state := &RegisterProfileInput{Path: initial.Path}
    fields := []huh.Field{
        pathDisplayNote(&state.Path, "Profile directory picked in the previous step"),
    }
    form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(styles.HuhTheme())
    return form, state, func(*huh.Form) any { return *state }, fields
}
```

Same shape for `buildCreateProfile` (`[Name, Note]`) and
`buildRegisterProject` (`[Name, Note, EnabledAgents]`). Production
`New*` wrappers ignore the fields slice — it exists only so tests can
type-assert without reflecting into `huh.Form` internals.

## Execution order

1. **Refactor helper.** Rename `readOnlyPathInput` → `pathDisplayNote`
   in `fields.go`, adjust body to return `*huh.Note`. Compilation will
   break at the three call sites.
2. **Update build helpers.** In each of `register_profile.go`,
   `create_profile.go`, `register_project.go`: replace the call site
   and extend the return tuple with `fields []huh.Field`.
3. **Update existing tests.** Every `buildX` caller in the modals tests
   uses `_, state, _ := build...` / `form, _, extract := build...`. Add
   a trailing `_` for the new return. Rewrite the three
   `Test*_PathReadOnly` tests as three focused tests each (see next
   section).
4. **Run gates.** `make fmt && make lint && make test`. `make build`
   confirms the binary still links.
5. **Update docs.** Edit the building-block paragraph, add the
   changelog entry.
6. **Manual smoke.** Follow the three smoke paths listed in
   description.md's Verification section.

## Tests (per modal)

Existing behaviour tests (prefill, submit, cancel, stable id, required
field rejection) stay unchanged aside from the tuple-length fix on the
build call.

Add these focused tests per form:

- `TestBuildRegisterProfile_PathFieldIsNote` — call
  `buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})`, walk
  the returned `fields` slice, assert `fields[0]` is `*huh.Note`, not
  `*huh.Input`. Also assert the rendered `form.View()` contains
  `"/tmp/seed"` so the picked path is visible.
- `TestBuildRegisterProfile_RuneKeyLeavesPathUnchanged` — build the
  form, `form.Init()`, feed `tea.KeyPressMsg{Text: "x", Code: 'x'}` via
  `form.Update`, assert `state.Path == "/tmp/seed"`.
- `TestBuildRegisterProfile_EnterCompletesForm` — build with a valid
  seed, `form.Init()`, feed `tea.KeyPressMsg{Code: tea.KeyEnter}`, drain
  the resulting `NextField` command via the existing `drainCmd` helper,
  assert `form.State == huh.StateCompleted`.

Analogous triples for `create_profile` and `register_project`:

- `create_profile` Enter-completes test seeds `Name: "demo"` on
  `CreateProfileInput` so the Name validator passes; drives the form
  through `submitForm` (already available) which walks fields via
  `NextField`. Assert `form.State == huh.StateCompleted`. The Note is
  the last field; `Group.nextField` recognises `Note.Skip()==true` when
  it's not the sole field, hits `OnLast`, dispatches `nextGroup`, and
  the form completes.
- `register_project` mirrors this — seed valid Name + EnabledAgents,
  drive through `submitForm`, assert completion. Rune-input test
  additionally asserts `state.EnabledAgents` (bound to the MultiSelect)
  is untouched by a rune fed while the Note or Name is focused — that
  keeps the "no key bleed-through" guarantee explicit for the multi-
  field case.

The rune-input tests use `form.Update` directly with a synthetic
`tea.KeyPressMsg`; no `submitForm` pump needed. This aligns with the
"Assert Behavior, Not Mock Mechanics" rule in `testing.md` — the
observation is: the pointer-bound state slot is not mutated.

## Docs & changelog

- `docs/architecture/05-building-block-view.md` — the sentence
  > "the follow-on form shows the picked path as a read-only field"

  becomes
  > "the follow-on form shows the picked path as a display-only `huh.Note`
  > row (no runtime edits, Enter advances the form)".

- New changelog: `docs/changelog/2026-07-22_0040-path-selector-reopens-on-enter-in-profile-form.md`.
  One short section describing the swap, why `huh.Input` did not work
  (no runtime read-only mode), and pointing at the three modals.

## Verification gate

- `make build && make lint && make test` all green.
- `go test ./internal/tui/modals/...` covers the new tests.
- Manual smoke — Register Profile, Create Profile, Register Project
  (paths in description.md Verification section). Picker must NOT
  re-open on Enter; typing runes must leave the picked path visible
  and unchanged.

## Risks / notes

- `huh.Note.Update` unconditionally returns `NextField` on any
  `tea.KeyPressMsg` that does not match its Prev/Next/Submit bindings.
  Users cannot type-and-see anything into the path row; that is the
  intended fix, not a regression.
- In the two multi-field forms, `Note.Skip()==true` means focus never
  visibly lands on the path row. Users tabbing past see Name → agents
  directly. This matches the intent — the path is display context,
  not input.
- `Description` on `huh.Note` runs the built-in mini markdown renderer
  (`field_note.go:render`). Paths that contain `*`, `_`, `` ` ``, or
  `\` will be interpreted. Two escape strategies exist if this bites
  in practice: switch to `Title(*value)` (bypasses `render`) or
  pre-escape the string. Not a known problem in the target home / repo
  paths, so left as follow-up if a report surfaces.
