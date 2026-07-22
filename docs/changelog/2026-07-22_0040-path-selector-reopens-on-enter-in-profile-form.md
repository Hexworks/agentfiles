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
advances the group without leaking. The three `build*` helpers now also
return `[]huh.Field` alongside the form, so tests can assert the field
type without reflecting into `huh.Form` internals.

## Decisions

- Return `[]huh.Field` from the three `build*` helpers — **Why:** the
  new tests need to assert `fields[i]` is `*huh.Note` (not `*huh.Input`).
  Adding an accessor slice keeps the assertion boring; the alternative
  is reflecting into `*huh.Group` fields, which is fragile.
- Keep `huh.Note` plain (no `.Next(true).NextLabel(...)`) — **Why:** the
  fix does not require an explicit "Next" button; the Note title +
  description plus focus flow already communicate that the row is
  display-only. If usability testing later shows the affordance is
  unclear, adding an explicit button is a one-line follow-up.

Considered but skipped:

- Custom `focus.Handler`-based skip (as used by the Edit Asset "Type"
  field) — needs a bespoke focus manager and does not compose with the
  plain `huh.Form` used in these modals. `huh.Note` already provides the
  needed behavior out of the box.

## Assumptions

- Path strings picked by the pathselector do not need to be escaped
  against `huh.Note`'s built-in mini-markdown renderer (`*`, `_`, `` ` ``,
  `\`) — **Why:** typical repo/home paths do not contain those runes;
  escaping is a follow-up if a real report surfaces.

## Other Notes

- `docs/architecture/05-building-block-view.md` updated: the
  `tui/modals/pathselector` section now describes the follow-on form as
  using a display-only `huh.Note`, not a read-only Input.
- No ADRs added — the change is a local bug fix, not an architectural
  decision.
- Test coverage grew from one `Test*_PathReadOnly` per modal to three
  focused tests per modal: `*_PathFieldIsNote`, `*_RuneKeyLeaves…Unchanged`,
  `*_EnterCompletesForm`. The register-project rune test additionally
  asserts `state.EnabledAgents` is untouched — that keeps the
  no-key-bleed guarantee explicit for the multi-field case.

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
func pathDisplayNote(value *string, description string) *huh.Note {
    return huh.NewNote().
        Title("Path").
        Description(description + "\n" + *value)
}
```

## Extend `build*` return with `[]huh.Field` (all three modals)

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
// after — fields slice exposed to tests only; production wrappers discard it.
func buildRegisterProfile(initial RegisterProfileInput) (*huh.Form, *RegisterProfileInput, func(*huh.Form) any, []huh.Field) {
    state := &RegisterProfileInput{Path: initial.Path}
    fields := []huh.Field{
        pathDisplayNote(&state.Path, "Profile directory picked in the previous step"),
    }
    form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(styles.HuhTheme())
    return form, state, func(*huh.Form) any { return *state }, fields
}
```

`buildCreateProfile` and `buildRegisterProject` follow the same shape:
the fields slice is `[nameInput, pathDisplayNote]` and
`[nameInput, pathDisplayNote, enabledAgentsSelect]` respectively.
