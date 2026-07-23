---
id: 0041
type: bug
status: Pending
topics: tui, security
depends_on: 0040
---

# Strip C0 controls (and DEL) from rendered path strings

Follow-up from task 0040 review. Task 0040 fixed the visible corruption
of paths containing `*`, `_`, `` ` ``, or `\` by moving the picked path
from `huh.Note.Description` (which runs a mini-markdown pass) into
`huh.Note.Title` (which does not). That closes the observable failure
mode users hit today, but leaves a pre-existing weakness in every
Charm/huh field, including `huh.Input`: `render`'s `default` branch
emits any other rune literally, so raw ESC (`\x1b`) and other C0
control characters (`0x00–0x1F` except `\t` / `\n`) survive into the
rendered view and can move the cursor, change colours, or corrupt the
frame around the modal.

## Context

- Path strings originate from the pathselector (`internal/tui/modals/pathselector`)
  and reach the follow-on form via `pathDisplayNote` in
  `internal/tui/modals/fields.go`.
- After task 0040 the value is placed in `Title(value)` on `huh.Note`.
  Title bypasses `render`, but the underlying strings are still raw
  filesystem bytes.
- The same class of runes reaches every other huh field where the
  user's input or a seed value is rendered — this is not a
  pathselector-specific concern.

## Scope

- Add a small sanitiser (working name: `sanitizeForDisplay(s string) string`
  under `internal/tui/modals/` or a shared helper package) that strips
  C0 controls `0x00–0x1F` (except `\t` and `\n`) and `0x7F` (DEL) from
  any string that will be rendered inside a huh field.
- Apply the sanitiser to `pathDisplayNote`'s `value` and to any other
  callable path/name rendering that accepts external input.
- Add unit tests covering: raw ESC, NUL, DEL, mixed-ASCII/UTF-8 paths.
  Verify that `\t` and `\n` are preserved.

## Out of scope

- The markdown-escape issue for `*` / `_` / `` ` `` / `\` — already
  closed by task 0040 by moving the value out of the markdown-rendered
  Description.
- Any change to the pathselector itself. The picker returns whatever
  the filesystem gave it; sanitising happens at the render boundary.

## Acceptance Criteria

- [ ] Sanitiser removes C0 controls (`0x00–0x1F` except `\t`/`\n`) and
      DEL (`0x7F`) from any string rendered inside a huh field.
- [ ] `pathDisplayNote` runs the sanitiser on its `value` argument.
- [ ] Other display-only renderings of external strings inside
      `internal/tui/modals/` are audited and updated as needed.
- [ ] Unit tests cover raw ESC, NUL, DEL, mixed-ASCII/UTF-8 paths, and
      the preservation of `\t` / `\n`.
- [ ] `make build && make test && make lint` all green.
