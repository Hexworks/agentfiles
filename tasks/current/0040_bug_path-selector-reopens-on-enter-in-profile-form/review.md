# Path selector re-opens on Enter in profile form — review

The DoD gate passed: every acceptance criterion has diff evidence, and no diff hunk falls outside the criteria or a declared refactor allowance. The swap `readOnlyPathInput` → `pathDisplayNote` fixes the reported symptom at the right layer and stays inside `internal/tui/modals/`. No domain packages or renderer/sync paths touched. Architecture doc and changelog match the actual change.

Seven issues surfaced across the seven reviewers, clustered around three real concerns and one test-quality cluster:

1. The `build*` helpers grew a fourth return (`[]huh.Field`) purely to give tests a positional handle — production callers all discard it with `_`. Flagged by Clean Code, Clean Architecture, SOLID (SRP + ISP), and Go.
2. `pathDisplayNote(value *string, ...)` takes a pointer, dereferences it once at construction, and discards it. The signature mirrors `nameInput` / `pathInput` but has none of their binding semantics — misleading. Flagged by Clean Code and Go.
3. The picked path is stuffed into `Description(description + "\n" + *value)`. `huh.Note.Description` runs a mini-markdown renderer that interprets `_`, `*`, `` ` ``, and `\`. The plan and changelog both acknowledge this and defer it. Flagged by Security, Clean Code, and Go — a folder called `my_repo` already renders wrong.
4. The `*_EnterCompletesForm` tests use `submitForm` → `form.NextField()`, which dispatches `nextFieldMsg` directly and bypasses the entire `KeyPressMsg → Group.Update → Note.Update` path — the exact path that carried the bug. Multiple test-shape mismatches around AC 5 / AC 6 and the register-project rune assertion.

Pick one solution per issue below and run `af.task.review-apply 0040` in a fresh session to have the fixes applied.

## Test-only `[]huh.Field` return pollutes production signature

> [!WARNING]
>
> - [Clean architecture](docs/guidelines/clean_architecture.md) — Introduce interfaces at boundaries only when they pay for themselves; don't shape production signatures for test observability.
> - [Clean code](docs/guidelines/clean_code.md) — Data And Objects: hide internal structure when callers should not rely on it; Needless complexity.
> - [SOLID](docs/guidelines/solid.md) — SRP: `build*` now does "build form" AND "expose fields for tests". ISP: production callers depend on a return value they never use.
> - [Go](docs/guidelines/go.md) — Keep packages cohesive; don't extend production signatures for test convenience when a smaller alternative exists.

`buildRegisterProfile` (`internal/tui/modals/register_profile.go:22`), `buildCreateProfile` (`internal/tui/modals/create_profile.go:25`), and `buildRegisterProject` (`internal/tui/modals/register_project.go:30`) now return a 4-tuple whose fourth element (`[]huh.Field`) has no production consumer. Every production caller discards it:

```go
// register_profile.go:18, create_profile.go:21, register_project.go:26
form, _, extract, _ := buildRegisterProfile(initial)
form, _, extract, _ := buildCreateProfile(initial)
form, _, extract, _ := buildRegisterProject(initial)
//                 ^ always `_` outside tests
```

The changelog admits it openly (`docs/changelog/2026-07-22_0040-…md:26-30`): *"the new tests need to assert `fields[i]` is `*huh.Note`(not`_huh.Input`). Adding an accessor slice keeps the assertion boring."_ Two side effects:

- The other three `build*` helpers in the package (`buildCreateAsset`, `buildEditProject`, `buildCreateFile`) still use the 3-tuple. Two shapes now coexist in one 200-line package.
- `fields[1].(*huh.Note)` is a positional assertion. Reorder the fields slice and the tests break on index, not on the property they claim to check.

Pick one:

- [ ] Move the test-only accessor behind an `export_test.go` (or extend `testhelpers_test.go`) helper — e.g. `func firstField(f *huh.Form) huh.Field { ... }` reaching into `huh.Group` via a package-private shim. Revert the three `build*` signatures to 3-tuples and delete every trailing `_` at the production call sites.
- [x] Collapse the 4-tuple into a named struct — `type builtRegisterProfileForm struct { Form *huh.Form; State *RegisterProfileInput; Extract func(*huh.Form) any; Fields []huh.Field }` (and analogous types per modal). Production wrappers still ignore `.Fields`, but the tuple shape stops leaking test intent through positional discards.
- [ ] Drop the type-assertion approach entirely and lean on behavioural assertions: the `_PathFieldIsNote` tests already check that `form.View()` contains the picked path. Add a companion assertion that the rendered view does **not** contain the `huh.Input` prompt glyph, and delete both the `fields` return and the `.(*huh.Note)` check. Revert the `build*` signatures to 3-tuples.

## `pathDisplayNote(value *string, ...)` pointer parameter is misleading

> [!WARNING]
>
> - [Clean code](docs/guidelines/clean_code.md) — Understandability: make intent visible in names, types, and function boundaries; Naming: types should describe domain meaning, not shape.
> - [Go](docs/guidelines/go.md) — Prefer explicit types; API shape should reflect actual semantics.

`internal/tui/modals/fields.go:45-49`:

```go
func pathDisplayNote(value *string, description string) *huh.Note {
    return huh.NewNote().
        Title("Path").
        Description(description + "\n" + *value)  // dereferenced once, snapshotted
}
```

Every other helper in the same file (`nameInput`, `pathInput`, `idInput`, `descriptionText`, `enabledAgentsSelect`) takes `value *string` because the underlying `huh.Input`/`MultiSelect` **binds** to the pointer and mutates it as the user types. `pathDisplayNote` dereferences the pointer exactly once at construction, embeds the string, and never touches the pointer again. The plan itself (`plan.md:63-65`) says: *"`*string`signature mirrors`nameInput`/`pathInput` for call-site uniformity. The value is read once at construction."\*

Consequences:

- A caller reading `pathDisplayNote(&state.Path, …)` reasonably expects the same "reads back into state" contract as `nameInput(&state.Name, …)`, and gets none.
- If a future maintainer mutates `state.Path` between `build*` and `form.Init()` (e.g. a retry flow re-seeds the path after an error), the Note description silently stays stale.
- Passing `nil` compiles and panics with no useful message. A value type rules it out at compile time.

Pick one:

- [x] Change the signature to `pathDisplayNote(value string, description string) *huh.Note` and update the three call sites (`create_profile.go:29`, `register_profile.go:25`, `register_project.go:38`) to pass `state.Path` directly. Update the doc comment to name the snapshot semantics explicitly.
- [ ] Keep `*string` but rewrite the doc comment to lead with "snapshot at construction — pointer is read once and then discarded" so the mismatch with other helpers is loud; add an `if value == nil { panic("nil path pointer") }` at the top so the failure mode is a named error, not a generic nil deref.

## Picked path lives in `Description` and hits the mini-markdown renderer

> [!WARNING]
>
> - [Security](docs/guidelines/security.md) — Treat external input as untrusted: filesystem paths carry rune classes that the renderer interprets, and the swap is a new attack surface — `huh.Input.View()` did not run description through the markdown pass; `huh.Note.View()` does.
> - [Clean code](docs/guidelines/clean_code.md) — Understandability / Opacity: `Title("Path")` holds no path; the actual value is jammed into `Description` with a `\n`. The field name no longer matches its content.
> - [Go](docs/guidelines/go.md) — Behavior should not silently corrupt user data; document renderer semantics if the value survives them.

`internal/tui/modals/fields.go:46-48`:

```go
return huh.NewNote().
    Title("Path").
    Description(description + "\n" + *value)  // *value is a filesystem path
```

`huh.Note.View()` renders the Description through a mini-markdown pass (`field_note.go:render`) that interprets:

- `*` → toggles bold ANSI
- `_` → toggles italic ANSI
- `` ` `` → toggles a codeblock (grey-on-black + leading space, `\x1b[0m` on close)
- `\` → escape prefix; next rune emitted literally and consumed

The plan (`plan.md:184-188`) and changelog (Assumptions, lines 45-48) both flag this and defer it as follow-up "if a report surfaces". But `~/.config/my_repo` or `~/repos/some_dir` already renders wrong — underscore is one of the most common characters in real folder names.

Independently: `huh.Input.Value(...)` (the previous implementation) rendered the value through `textinput.View()` — no markdown pass. So the swap **introduces** this exposure surface; it did not inherit it. `render()`'s `default` branch also emits any other rune literally, including raw ESC (`\x1b`) — a pre-existing weakness of both `Input` and `Note`, worth noting as an independent tracker.

Pick one:

- [ ] Move the path into `Title(*value)` (Title is rendered via `NoteTitle.Render(wrap(...))` — no markdown pass) and put the "picked in the previous step" sentence into `Description(description)`. Trades layout for correctness; the field name now matches its content.
- [ ] Keep the current layout but pre-escape `*`, `_`, `` ` ``, `\` in `*value` before concatenation. A small `escapeHuhMarkdown(s string) string` helper next to `pathDisplayNote` covers it. Add a unit test that seeds `Path: "/tmp/some_dir"` and asserts `form.View()` contains the literal `some_dir` (no italic ANSI wrapping it).
- [x] Do both: put the path in `Title` **and** file a follow-up task to strip C0 controls (`0x00–0x1F` except `\t`/`\n`) and DEL from any path before rendering. Belt-and-braces; the follow-up task closes the pre-existing ESC issue for other fields too.

## `*_EnterCompletesForm` tests use `NextField()` pump, bypass Enter key routing

> [!WARNING]
>
> - [Testing](docs/guidelines/testing.md) — Assert Behavior, Not Mock Mechanics: don't mock the code path the test is supposed to verify.

The three `*_EnterCompletesForm` tests (`register_profile_test.go:93-100`, `create_profile_test.go:106-113`, `register_project_test.go:150-161`) are named for the reported bug — Enter on the single-Note group failing to reach `StateCompleted` and leaking back to the shell as a "select-path" trigger — but they do not exercise that path.

`submitForm` (`internal/tui/modals/testhelpers_test.go:35-52`) drives the form via `form.NextField()`, which internally injects a `nextFieldMsg{}` directly into `form.Update`. That message bypasses the entire `tea.KeyPressMsg` dispatch chain in `Form.Update` → `Group.Update` → `Note.Update`. The bug lived in exactly that chain: `huh.Input` did not return `NextField` on the Enter _keypress_ for a single-field group, whereas `huh.Note.Update` does.

```go
// register_profile_test.go:93-100
func TestBuildRegisterProfile_EnterCompletesForm(t *testing.T) {
    form, _, _, _ := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})
    submitForm(t, form)   // feeds nextFieldMsg, NOT tea.KeyPressMsg{Enter}
    if form.State != huh.StateCompleted {
        t.Fatalf("form.State = %v, want StateCompleted", form.State)
    }
}
```

If someone swapped `pathDisplayNote` back to `huh.NewInput()`, or changed `Note.Update` to return `nil` on Enter, this test would still go green — `nextFieldMsg → nextGroupMsg → StateCompleted` is a separate code path from the keypress-to-`NextField` mapping the bug depended on. AC 5 in `description.md` explicitly says "feeds `Enter`" — the test does not feed Enter.

Pick one:

- [x] Rewrite the three tests to feed `tea.KeyPressMsg{Code: tea.KeyEnter}` via `form.Update` and drain the returned command through the existing `drainCmd` helper. Assert `form.State == huh.StateCompleted`. Keep or delete the `submitForm`-based version — it is a broader wiring smoke, not the regression guard.
- [ ] Add companion tests that feed `tea.KeyPressMsg{Code: tea.KeyEnter}` alongside the existing `submitForm` variants (do not replace). Name them `*_EnterKeyCompletesForm` to distinguish. Keeps both signals.
- [ ] Rename the current tests to `*_NextFieldPumpCompletesForm` (accurate for what they assert), and add the KeyEnter tests separately as above so the regression names are visibly distinct.

## AC 6 wording is not achievable in a multi-field group with a `huh.Note`

> [!WARNING]
>
> - [Testing](docs/guidelines/testing.md) — Test One Behavior At A Time: names and assertions should describe an achievable behaviour.

AC 6 in `description.md` says: _"For create-profile (Name + Note), a unit test seeds a valid Name, advances focus off the Name field, feeds Enter on the Note row, and asserts `form.State == huh.StateCompleted`."_

In `create_profile.go` the group is `[nameInput, pathDisplayNote]`. `huh.Note.Skip()` returns `true` when the Note is **not** the sole field in its group (see `field_note.go:WithPosition`). `Group.nextField` walks past skipped fields, so focus never lands on the Note — the walk goes Name → (skip Note) → `nextGroup`. There is no path in the current code that "feeds Enter on the Note row" in the create-profile form.

The written `TestBuildCreateProfile_EnterCompletesForm` at `create_profile_test.go:106-113` does not attempt this — it runs `submitForm` (which uses `NextField()` — see the previous issue). The AC and the test are misaligned in opposite directions.

Pick one:

- [ ] Rewrite AC 6 in `description.md` to match reality: "For create-profile (Name + Note), a unit test seeds a valid Name, drives the form to completion, and asserts `form.State == huh.StateCompleted` — Note is skipped in multi-field groups, so focus never visibly lands on the path row." Update the test comment at `create_profile_test.go:101-105` similarly.
- [x] Keep AC 6's intent by adding a synthetic single-Note test: build a bespoke `huh.Form` containing only `pathDisplayNote(&s, "…")`, feed `tea.KeyPressMsg{Code: tea.KeyEnter}`, assert `StateCompleted`. This validates `Note.Update`'s Enter-branch under the "sole field" position — the exact register-profile shape — without lying about create-profile's field layout.

## The "leak back to shell" symptom is never asserted at unit level

> [!WARNING]
>
> - [Testing](docs/guidelines/testing.md) — Assert Behavior, Not Mock Mechanics: behaviour under test should be the observation.

The user-visible bug in `description.md` and the changelog is: "The keystroke bubbled out of the form and the shell routed it as a 'select-path' trigger again." No test in `internal/tui/modals/` asserts that `form.Update(KeyEnter)` **consumes** the keypress — that no `tea.KeyPressMsg` re-surfaces to the caller after routing.

`Form.Update` on `KeyPressMsg` handles `f.keymap.Quit` (`form.go:562-569`) and otherwise delegates to `Group.Update` → `Note.Update`. Consumption is observable at the boundary: drain the returned `tea.Cmd` and check the drained message stream contains `nextFieldMsg` / `nextGroupMsg` / `ResolvedMsg` and never a raw `tea.KeyPressMsg`.

Pick one:

- [x] Add a single regression test in `register_profile_test.go` (single-Note group, exactly the buggy shape) that feeds `tea.KeyPressMsg{Code: tea.KeyEnter}`, drains via `drainCmd`, then asserts both `form.State == huh.StateCompleted` and that no drained message is a `tea.KeyPressMsg`. This is the load-bearing regression guard the AC list implies but does not have.
- [ ] Alternative that mirrors what the shell actually sees: wrap the form in `modal.NewForm`, drive `m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})`, drain, and assert the resulting `modal.ResolvedMsg.Confirmed == true`. This is the exact boundary where the leak happened; closer to the reported symptom.

## `TestBuildRegisterProject_RuneKeyLeavesStateUnchanged` asserts a vacuous invariant

> [!WARNING]
>
> - [Testing](docs/guidelines/testing.md) — Assert Behavior, Not Mock Mechanics: the assertion should be about the behaviour implemented by the unit under test.

`register_project_test.go:123-143` feeds rune `x` after `form.Init()` and asserts `state.Path` and `state.EnabledAgents` are unchanged:

```go
seedAgents := []string{AgentClaudeCode}
form, state, _, _ := buildRegisterProject(RegisterProjectInput{
    Name:          "Demo",
    Path:          "/repos/seed",
    EnabledAgents: seedAgents,
})
form.Init()

drainCmd(form, func() tea.Cmd {
    _, cmd := form.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
    return cmd
}())

if state.Path != "/repos/seed" { ... }
if !reflect.DeepEqual(state.EnabledAgents, seedAgents) { ... }
```

After `form.Init()` focus is on the first field — `nameInput`, a `huh.Input`. `Group.Update` (`group.go:260-278`) routes `tea.KeyPressMsg` only to the currently focused field. Rune `x` goes into the Name Input and mutates `state.Name` from `"Demo"` to `"Demox"`. Neither the Note nor the MultiSelect receives the keypress — the `state.Path` assertion is unfalsifiable by the Note swap, and the `state.EnabledAgents` assertion is unfalsifiable by anything the test could reasonably regress. The comment claims "no key bleed-through in the multi-field case either", but the case exercised does not correspond to that claim.

Pick one:

- [ ] Advance focus past the Name field before feeding the rune. Call `form.NextField()` once (with the valid seeded Name) so focus lands on `EnabledAgents` (Note is skipped), then feed `x` and assert `state.EnabledAgents` is unchanged. That is the "MultiSelect ignores runes" claim from the comment, and it is the actual behaviour the fix delivers. While at it, replace the anonymous-closure wrapper `drainCmd(form, func() tea.Cmd { _, cmd := form.Update(...); return cmd }())` with the boring two-liner `_, cmd := form.Update(...); drainCmd(form, cmd)` — apply to all three `*_RuneKey…` tests.
- [x] Split into two focused tests: one for the single-Note case in register-profile (focus is on the Note, rune must not mutate `state.Path`), and one for the multi-field case that focuses past the Note onto `EnabledAgents` and asserts the MultiSelect rejects runes. Delete the current combined assertion — it exercises neither claim.
