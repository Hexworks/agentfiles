---
id: 0041
type: bug
status: in-review
topics: tui, security
depends_on: 0040
---

# Neutralise C0 controls (and DEL) in rendered path strings

Follow-up from task 0040 review. Task 0040 fixed the visible corruption
of paths containing `*`, `_`, `` ` ``, or `\` by moving the picked path
from `huh.Note.Description` (which runs a mini-markdown pass) into
`huh.Note.Title` (which does not). That closes the observable failure
mode users hit today, but leaves a pre-existing weakness: the underlying
strings placed in `Title(...)` are still raw filesystem bytes, so raw
ESC (`\x1b`) and other C0 control characters can survive into the
rendered view and move the cursor, change colours, or corrupt the frame
around the modal.

## Approach: reuse `styles.Safe`, do not invent a new sanitiser

A terminal-safe sanitiser already exists and is the single source of
truth for this exact concern: `styles.Safe(s string) string` in
`internal/tui/styles/styles.go`. It is public, already imported by
`internal/tui/modals`, and covers **more** than C0/DEL — it also rejects
C1 controls, bidi overrides/isolates, and zero-width formatters.

Its strategy is quote-the-whole-string, not per-rune stripping: as soon
as one hostile rune is detected the entire value is returned via
`strconv.Quote`, so the user sees the literal escaped form
(`"~/re\x1bpo"`) rather than a silently-cleaned path that no longer
matches the real bytes. For a security fix this is the safer primitive —
nothing is hidden. `styles.Safe` is already the sanitiser used by the
status bar and notifications views.

Decision: **do not** add a new `sanitizeForDisplay` helper. Wrap the
rendered path value in `styles.Safe`.

## Context

- Path strings originate from the pathselector
  (`internal/tui/modals/pathselector`) and reach the follow-on form via
  `pathDisplayNote` in `internal/tui/modals/fields.go`.
- After task 0040 the value is placed in `Title(value)` on `huh.Note`.
  `Title` bypasses the markdown renderer but the string is still raw.
- `pathDisplayNote` is the **only** non-literal `Title`/`Description`
  argument in `internal/tui/modals`; every other field description is a
  static literal. So `value` is the sole external string in this package
  that reaches a display-only render.

## Scope

- Wrap the `value` argument of `pathDisplayNote`
  (`internal/tui/modals/fields.go`) in `styles.Safe` at the `Title(...)`
  call site.
- Backfill unit tests for `styles.Safe` in
  `internal/tui/styles/styles_test.go` — it is currently untested and
  this fix depends on its behaviour.
- Add one wiring test in `internal/tui/modals` proving `pathDisplayNote`
  actually runs its value through `styles.Safe`.

## Out of scope

- The markdown-escape issue for `*` / `_` / `` ` `` / `\` — already
  closed by task 0040.
- Any change to the pathselector itself. The picker returns whatever the
  filesystem gave it; sanitising happens at the render boundary.
- **Editable** huh fields (`huh.Input` seed values such as an existing
  path pre-filled in edit/register forms). Their `Value(*string)` binding
  is owned by huh's own render loop; pre-sanitising a bound editable value
  would mutate what the user is typing. The task description's "every huh
  field" is the *motivation* for why control runes matter, not a mandate
  to rewrite editable fields. This deferral also covers the **initial
  display** of persisted seeds: `edit_project.go` seeds `pathInput` from
  `initial.Path` (sourced from `~/.agentfiles/projects.json`, which is
  hand-editable / Adopt-populated external data), and bubbles/textinput
  echoes the seed verbatim, so a raw control rune persisted in a project
  path would render into the Edit modal frame before any keystroke. This
  is a known, deliberate limit — narrower than the `\t`/`\n` residual
  below (it needs a hostile `projects.json`) and sanitising a bound
  editable value is out of scope for the reasons above.
- Any new bespoke handling of `\n` / `\t`. `styles.Safe` preserves both
  (they are in its allow-switch). A filesystem path may legally contain a
  newline, so a Note `Title` could render multiline and mildly shift the
  modal frame. This is an **accepted residual**: it is not a
  control-sequence hijack (the actual harm), it is extraordinarily rare,
  and diverging from `styles.Safe`'s contract for one call site is not
  worth it. Documented here so it is a known, deliberate limit.

## Acceptance Criteria

- [ ] `pathDisplayNote` wraps its `value` in `styles.Safe` before placing
      it in `Title(...)`; no new sanitiser function is introduced.
- [ ] `styles.Safe` unit tests (in `internal/tui/styles/styles_test.go`)
      cover: raw ESC, NUL, DEL, a C1 control, a bidi override, and a
      zero-width formatter all force the quoted-literal form; a plain
      mixed ASCII/UTF-8 path is returned unchanged; `\t` and `\n` in an
      otherwise-safe string are preserved.
- [ ] A `pathDisplayNote` wiring test builds a form with a hostile path
      value (e.g. `~/re\x1bpo`), renders the form view, and asserts the
      view contains the escaped literal `\x1b` (proving `styles.Safe`
      ran) and does not emit the raw injected control sequence.
- [ ] `make build && make test && make lint` all green.

## Verification

- `go test ./internal/tui/styles -run TestSafe` — hostile-rune (raw ESC,
  NUL, DEL, C1, bidi, zero-width), clean-path, tab/newline, and
  empty-string cases all pass.
- `go test ./internal/tui/modals -run TestPathDisplayNote` — wiring test
  asserts the rendered `form.View()` contains the escaped literal `\x1b`
  (proving `styles.Safe` ran) and never the raw `re\x1bpo` bytes; clean
  path renders verbatim.
- `make build && make test && make lint` all green.
