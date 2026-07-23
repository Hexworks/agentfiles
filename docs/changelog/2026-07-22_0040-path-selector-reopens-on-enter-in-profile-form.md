# 0040 changes

Fix bug introduced by task 0039's two-step pathselector → form flow: in
Register Profile, Create Profile, and Register Project, the path field in
the follow-on form was still editable and, in the single-field
register-profile case, pressing Enter re-opened the pathselector instead
of submitting.

Root cause: `internal/tui/modals/fields.go:readOnlyPathInput` built a
plain `huh.NewInput` with `" (read-only)"` appended to the description
string. `huh.Input` has no runtime read-only mode — the field kept
consuming runes, and in a single-field group its Enter did not advance
the form to `StateCompleted`. The keystroke bubbled out of the form and
the shell routed it as a "select-path" trigger again.

Swap the helper to `pathDisplayNote`, which returns a `*huh.Note`. Note
has no value binding (`GetKey()` returns `""`), so runes cannot mutate
the path; `Note.Update` unconditionally returns `NextField` on any
keypress that is not a Prev/Next/Submit binding, so Enter (or any rune)
advances the group without leaking. The three `build*` helpers now
return a named struct (`builtRegisterProfileForm`, `builtCreateProfileForm`,
`builtRegisterProjectForm`) that carries the form, backing state,
extract callback, and the fields slice; tests reach into `.State` /
`.Fields`, production callers use `.Form` and `.Extract`.

## Decisions

- Return a named `built*Form` struct from the three `build*` helpers —
  **Why:** the new tests need to assert `Fields[i]` is `*huh.Note` (not
  `*huh.Input`). A 4-tuple return with the fourth positional slot
  discarded by every production caller (`form, _, extract, _ := build…`)
  leaked test intent through the signature; a named struct with an
  explicit `Fields` slot keeps the intent visible and matches the code
  base's preference for explicit types over positional returns.
- Place the picked path in `Title(value)` rather than
  `Description(description + "\n" + *value)` — **Why:** `huh.Note.View()`
  renders `Description` through a mini-markdown pass that interprets
  `*`, `_`, `` ` ``, `\`. Common repo paths (`~/repos/my_repo`) rendered
  wrong. `NoteTitle.Render` bypasses that pass, so the path renders
  literally. Also lets the signature drop the pointer parameter — the
  Note has no runtime binding so the value is a snapshot at
  construction and passing `string` matches actual semantics.
- Keep `huh.Note` plain (no `.Next(true).NextLabel(...)`) — **Why:** the
  fix does not require an explicit "Next" button; the Note title +
  description plus focus flow already communicate that the row is
  display-only. If usability testing later shows the affordance is
  unclear, adding an explicit button is a one-line follow-up.
- Test `*_EnterCompletesForm` by feeding a real
  `tea.KeyPressMsg{Code: tea.KeyEnter}` through `form.Update` and
  draining the returned command via `drainCmd` — **Why:** the pre-review
  variant used `form.NextField()`, which dispatches `nextFieldMsg`
  directly and bypasses the entire `KeyPressMsg → Group.Update →
  Note.Update` path — the exact path the bug lived in. The rewritten
  test walks the real keypress dispatch chain.
- Add `TestBuildRegisterProfile_EnterConsumedNoLeak` — **Why:** the
  user-visible symptom was "Enter leaked back to the shell and
  re-triggered select-path". Consumption is observable at the form
  boundary; the new test collects drained messages via
  `drainCmdCollect` and asserts none of them are `tea.KeyPressMsg`.
- Add `TestPathDisplayNote_SoleFieldEnterCompletesForm` — **Why:** AC 6
  wording targeted `Note.Update`'s Enter branch at the "sole field in
  its group" position, but in create-profile the Note is not sole and
  `Note.Skip()` returns true so focus never lands on it. The synthetic
  test builds a single-Note form directly, exactly matching the buggy
  register-profile shape, without lying about create-profile's field
  layout.

Considered but skipped:

- Custom `focus.Handler`-based skip (as used by the Edit Asset "Type"
  field) — needs a bespoke focus manager and does not compose with the
  plain `huh.Form` used in these modals. `huh.Note` already provides the
  needed behavior out of the box.
- Escaping `*` / `_` / `` ` `` / `\` in the picked path — moving the
  value into `Title(value)` avoids the mini-markdown pass entirely and
  is simpler than a pre-escape helper.

## Follow-ups

- Task 0041 (backlog) — strip C0 controls (`0x00–0x1F` except `\t` /
  `\n`) and DEL from any string rendered inside a huh field. Charm's
  `render` default branch emits raw runes verbatim, so ESC and other
  C0 controls still survive into rendered views (this is a pre-existing
  weakness, not introduced by 0040).

## Other Notes

- `docs/architecture/05-building-block-view.md` already describes the
  follow-on form as using a display-only `huh.Note` row — no update
  needed for the Title/Description move because the paragraph does not
  specify which Note field carries the path.
- No ADRs added — the change is a local bug fix, not an architectural
  decision.
- Test coverage grew from one `Test*_PathReadOnly` per modal to a set
  per modal: `*_PathFieldIsNote`, `*_RuneKey…LeavesPathUnchanged`,
  `*_EnterCompletesForm`, plus the register-profile-only
  `_EnterConsumedNoLeak` regression guard, the create-profile-only
  synthetic `TestPathDisplayNote_SoleFieldEnterCompletesForm`, and a
  register-project multi-field `_RuneOnMultiSelectLeavesEnabledAgentsUnchanged`
  test that advances focus off Name onto EnabledAgents before feeding
  the rune (the pre-review test fed the rune while focus was still on
  Name, so both assertions were unfalsifiable by the Note swap).

## Swap `readOnlyPathInput` → `pathDisplayNote`

```go
// before
func readOnlyPathInput(value *string, description string) *huh.Input {
    return huh.NewInput().
        Key("path").
        Title("Path").
        Description(description + " (read-only)").
        Value(value)
}
```

```go
// after — Note has no value binding; Enter/runes return NextField.
// Value is a snapshot (Note has no runtime binding). Path lives in
// Title(...) so it bypasses the mini-markdown renderer that
// Description runs on (`*`, `_`, `` ` ``, `\`).
func pathDisplayNote(value string, description string) *huh.Note {
    return huh.NewNote().
        Title(value).
        Description(description)
}
```

## Named `built*Form` struct return (all three modals)

```go
// before — buildRegisterProfile
func buildRegisterProfile(initial RegisterProfileInput) (*huh.Form, *RegisterProfileInput, func(*huh.Form) any) {
    state := &RegisterProfileInput{Path: initial.Path}
    form := huh.NewForm(
        huh.NewGroup(
            readOnlyPathInput(&state.Path, "Profile directory picked in the previous step"),
        ),
    ).WithTheme(styles.HuhTheme())
    return form, state, func(*huh.Form) any { return *state }
}
```

```go
// after — struct return keeps the fields slice named, not positional.
type builtRegisterProfileForm struct {
    Form    *huh.Form
    State   *RegisterProfileInput
    Extract func(*huh.Form) any
    Fields  []huh.Field
}

func buildRegisterProfile(initial RegisterProfileInput) builtRegisterProfileForm {
    state := &RegisterProfileInput{Path: initial.Path}
    fields := []huh.Field{
        pathDisplayNote(state.Path, "Profile directory picked in the previous step"),
    }
    form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(styles.HuhTheme())
    return builtRegisterProfileForm{
        Form:    form,
        State:   state,
        Extract: func(*huh.Form) any { return *state },
        Fields:  fields,
    }
}
```

`buildCreateProfile` and `buildRegisterProject` follow the same shape
with `builtCreateProfileForm` / `builtRegisterProjectForm`; the fields
slice is `[nameInput, pathDisplayNote]` and
`[nameInput, pathDisplayNote, enabledAgentsSelect]` respectively.
