# Plan — 0041 Neutralise C0 controls (and DEL) in rendered path strings

Task: [`./description.md`](./description.md) · type `bug` · topics `tui, security` · depends_on `0040` (done).

## Summary

`pathDisplayNote` (`internal/tui/modals/fields.go`) places a raw
filesystem path into a `huh.Note` `Title`. Raw ESC (`\x1b`) and other
control runes in that path survive into the rendered frame and can
hijack the terminal. The fix is a **one-line** change plus tests: wrap
the value in the already-existing, stronger terminal-safe sanitiser
`styles.Safe` (`internal/tui/styles/styles.go:274`). No new sanitiser is
introduced (grilling decision — see `description.md` "Approach").

Nothing here crosses an external boundary: no `os/exec`, no `filepath`,
no serialization the tool did not author. The work is pure in-memory
string sanitisation + in-process huh form rendering. So per
`docs/guidelines/testing.md` the cheapest useful tests are unit tests;
the "Cross-Boundary Integration" real-stack rule does not apply.

## Design decisions (resolved in refinement)

1. **Reuse `styles.Safe`, no new function.** It is public, already
   imported by `internal/tui/modals`, and covers C0 + DEL + C1 + bidi +
   zero-width. Strategy is quote-whole-string (`strconv.Quote`) on any
   hostile rune, so a hostile path renders as a visible escaped literal
   (`"~/re\x1bpo"`) rather than being silently stripped.
2. **Scope = display-only.** Only `pathDisplayNote`'s `value` (the sole
   non-literal `Title`/`Description` arg in the package). Editable
   `huh.Input` seed values are out — huh owns their bound render loop;
   pre-sanitising would mutate what the user types.
3. **`\t`/`\n` residual accepted.** `Safe` preserves both; a legal
   newline in a path can render a multiline Note title. Not a
   control-sequence hijack; documented, not handled bespoke.

## Execution plan

### Step 1 — Wire `styles.Safe` into `pathDisplayNote`

File: `internal/tui/modals/fields.go`.

- `pathDisplayNote` already lives in a package that imports
  `internal/tui/styles` (e.g. `create_profile.go:7`), but `fields.go`
  itself does not yet import it — add
  `"github.com/hexworks/agentfiles/internal/tui/styles"` to `fields.go`'s
  import block.
- Change the `Title(value)` call to `Title(styles.Safe(value))`.
- Update the `pathDisplayNote` doc comment: it currently explains only
  the markdown-bypass rationale (task 0040); add one sentence that the
  value is run through `styles.Safe` so C0/DEL/C1/bidi/zero-width runes
  cannot reach the frame (task 0041), and note `\t`/`\n` are preserved by
  design.

No other call site changes — the three callers
(`create_profile.go:38`, `register_profile.go:36`,
`register_project.go:48`) pass their path straight through and are
unaffected.

### Step 2 — Backfill `styles.Safe` unit tests

File: `internal/tui/styles/styles_test.go` (currently only tests
`SeverityLabel`; `Safe` is untested).

Add a table-driven `TestSafe_*` set. One behavior per row, names after
the outcome:

- Hostile runes force the quoted-literal form (result `!= input` and
  equals `strconv.Quote(input)`): raw ESC `\x1b`, NUL `\x00`, DEL
  `\x7f`, a C1 control (``), a bidi override (`‮`), a
  zero-width formatter (`​`).
- Clean input returned unchanged: a mixed ASCII/UTF-8 path such as
  `~/repos/münchen_project`.
- `\t` and `\n` inside an otherwise-safe string are preserved (result
  `== input`, still contains the tab/newline).
- Empty string returns empty (guards the `s == ""` fast path).

Assert against `strconv.Quote(input)` / identity — not against a
hand-copied expected literal — so the test cannot drift with the impl.

### Step 3 — `pathDisplayNote` wiring test

File: `internal/tui/modals/fields_test.go` (new) or an existing modals
test file. Model on the isolated single-Note pattern already in
`create_profile_test.go` (`TestPathDisplayNote_SoleFieldEnterCompletesForm`)
and the `form.View()` assertions in `register_profile_test.go:66`.

- Given: a hostile path value, e.g. `~/re\x1bpo`.
- When: build a form containing `pathDisplayNote(hostile, "picked")`,
  call `form.Init()`, read `form.View()`.
- Then: assert the view contains the escaped literal `\x1b` (the 4 chars
  `\`,`x`,`1`,`b` — verified: `strconv.Quote("~/re\x1bpo")` yields
  `"~/re\x1bpo"`), proving `styles.Safe` ran. A raw, unsanitised path
  would instead embed a real ESC byte and never the literal text.
- Complementary green-path assertion (may reuse existing tests): a clean
  path (`/tmp/seed`) still appears verbatim in the view — `Safe` leaves
  safe strings untouched, so `register_profile_test.go:66`,
  `create_profile_test.go:80`, `register_project_test.go:114` keep
  passing.

### Step 4 — Quality gate

`make build && make test && make lint` all green.

## ADRs / docs / guidelines

- **ADR:** none. Reusing an existing primitive; no durable architectural
  decision.
- **Docs:** none required. (Changelog entry is written by
  `af.task.implement`, not here.)
- **Guidelines:** no new/updated files.

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| `pathDisplayNote` places the path in `Title(value)` (bypasses markdown) | `internal/tui/modals/fields.go:48-52` | `return huh.NewNote().Title(value).Description(description)` |
| `styles.Safe` is public and quote-whole-string on any hostile rune | `internal/tui/styles/styles.go:274-284` | `for _, r := range s { if !isTerminalSafeRune(r) { return strconv.Quote(s) } } return s` |
| `Safe` preserves `\t` and `\n` (allow-switch) | `internal/tui/styles/styles.go:291-294` | `case '\n', '\t': return true` |
| `Safe` rejects C0, DEL, C1, bidi, zero-width | `internal/tui/styles/styles.go:295-313` | `if r < 0x20 || r == 0x7f { return false }` … C1/bidi/zero-width/`unicode.IsControl` branches |
| `modals` package already imports `internal/tui/styles` (no cycle) | `internal/tui/modals/create_profile.go:7` | `"github.com/hexworks/agentfiles/internal/tui/styles"` |
| `pathDisplayNote` is the only non-literal `Title`/`Description` arg in modals | grep of `internal/tui/modals/*.go` (non-test) | sole hit `fields.go:49` |
| `strconv.Quote("~/re\x1bpo")` yields text containing literal `\x1b` | Go stdlib `strconv` (external) — verified in repro shell | `"~/re\x1bpo"` (contains-literal-x1b: true) |
| `form.View()` includes a `pathDisplayNote` title in a built form | `internal/tui/modals/register_profile_test.go:66` | `if view := built.Form.View(); !strings.Contains(view, "/tmp/seed")` |
| `styles.Safe` currently has no unit test | `internal/tui/styles/styles_test.go` (whole file) | only `TestSeverityLabel_Vocabulary` present |

## Acceptance Criteria

- [ ] `internal/tui/modals/fields.go` `pathDisplayNote` calls
      `Title(styles.Safe(value))`; no new sanitiser function is added;
      the doc comment notes the `styles.Safe` pass and the preserved
      `\t`/`\n`.
- [ ] `TestSafe_*` in `internal/tui/styles/styles_test.go` proves: raw
      ESC, NUL, DEL, a C1 control, a bidi override, and a zero-width
      formatter each return `strconv.Quote(input)` (`!= input`); a mixed
      ASCII/UTF-8 path returns unchanged; a string with `\t`/`\n`
      (otherwise safe) is preserved; empty string returns empty. Expected
      values are `strconv.Quote`/identity, never hand-copied literals.
- [ ] `TestPathDisplayNote_SanitisesControlRunes` (modals): builds a form
      with a hostile path (`~/re\x1bpo`), calls `form.Init()`, and
      asserts `form.View()` contains the escaped literal `\x1b`, proving
      `styles.Safe` ran at the render boundary.
- [ ] A clean path still renders verbatim: existing
      `register_profile_test.go:66`, `create_profile_test.go:80`,
      `register_project_test.go:114` remain green (no regression from the
      `Safe` wrap).
- [ ] `make build && make test && make lint` all green.
