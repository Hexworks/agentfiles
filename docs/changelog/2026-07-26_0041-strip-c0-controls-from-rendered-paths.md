# 0041 changes

Neutralised C0 controls (and DEL/C1/bidi/zero-width) in the path string
that `pathDisplayNote` renders into a `huh.Note` title. Task 0040 moved
the picked path out of `Description` (markdown-rendered) into `Title`
(not markdown-rendered), closing the visible `*`/`_`/`` ` ``/`\`
corruption. But the string was still raw filesystem bytes, so a raw ESC
(`\x1b`) or other control rune in a path could survive into the rendered
frame and hijack the terminal (move the cursor, change colours, corrupt
the modal frame).

The fix wraps the value in the pre-existing terminal-safe sanitiser
`styles.Safe` at the single render boundary. No new sanitiser was
introduced. `styles.Safe` uses a quote-the-whole-string strategy: on any
hostile rune it returns `strconv.Quote(s)`, so the user sees the visible
escaped literal (`"~/re\x1bpo"`) instead of a silently-cleaned path — the
safer primitive for a security fix because nothing is hidden.

`styles.Safe` was previously untested; it is now backfilled with a
table-driven unit suite, and a modals wiring test proves the `Safe` pass
actually runs at the `pathDisplayNote` boundary.

## Decisions

- Reuse `styles.Safe`, do not invent a new `sanitizeForDisplay` helper —
  **Why:** `Safe` is public, already imported by `internal/tui/modals`,
  and covers more than C0/DEL (also C1, bidi overrides/isolates,
  zero-width formatters). It is already the sanitiser used by the status
  bar and notifications views; a second sanitiser would be a drift risk.
- Scope limited to `pathDisplayNote`'s `value` — the sole non-literal
  `Title`/`Description` argument in the `modals` package. **Why:** every
  other field description is a static literal; only the picked path is an
  external string reaching a display-only render.

Considered but deliberately not done:

- Sanitising editable `huh.Input` seed values — huh owns their
  `Value(*string)` bound render loop; pre-sanitising would mutate what
  the user is typing.
- Bespoke handling of `\t`/`\n` — `Safe` preserves both by design. A
  legal newline in a path may render a multiline Note title; this is an
  accepted, documented residual, not a control-sequence hijack.

## Assumptions

- `strconv.Quote("~/re\x1bpo")` yields text containing the literal chars
  `\x1b` — **Why:** Go stdlib contract; the wiring test asserts against
  the escaped literal rather than a hand-copied expected string, so it
  cannot drift from the impl.

## Other Notes

- No ADR: reuses an existing primitive, no durable architectural
  decision.
- No docs/guidelines changes.
- Test expectations use `strconv.Quote(input)`/identity, never
  hand-copied literals, so the suite tracks the implementation.

## `pathDisplayNote` runs the path through `styles.Safe`

`internal/tui/modals/fields.go` — the render boundary now sanitises.

```go
// before
func pathDisplayNote(value string, description string) *huh.Note {
	return huh.NewNote().
		Title(value).
		Description(description)
}
```

```go
// after — value passed through styles.Safe so control runes cannot reach the frame
func pathDisplayNote(value string, description string) *huh.Note {
	return huh.NewNote().
		Title(styles.Safe(value)).
		Description(description)
}
```

## Backfilled `styles.Safe` unit tests

`internal/tui/styles/styles_test.go` — `Safe` was untested; added a
table-driven suite covering hostile runes (raw ESC, NUL, DEL, C1, bidi
override, zero-width formatter) all forcing `strconv.Quote(input)`, a
clean mixed ASCII/UTF-8 path returned unchanged, `\t`/`\n` preserved, and
the empty-string fast path.

## Added `pathDisplayNote` wiring test

`internal/tui/modals/fields_test.go` (new) — builds a form with a hostile
path (`~/re\x1bpo`), renders `form.View()`, and asserts the view contains
the escaped literal `\x1b` (proving `Safe` ran) and never the raw
injected `re\x1bpo` byte sequence. A companion test asserts a clean path
(`/tmp/seed`) still renders verbatim.
