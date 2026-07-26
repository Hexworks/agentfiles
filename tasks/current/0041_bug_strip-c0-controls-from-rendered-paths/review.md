# strip-c0-controls-from-rendered-paths review

The fix itself is correct, minimal, and lands at the right architectural
boundary. `pathDisplayNote` now wraps its path value in `styles.Safe`
(`internal/tui/modals/fields.go:59`), reusing the established sanitiser
instead of inventing a new one; `styles.Safe`'s rune coverage (C0 / DEL /
C1 / bidi / zero-width) fully neutralises the terminal-injection threat at
this display edge, and the claim that `pathDisplayNote`'s `value` is the
only non-literal `Title`/`Description` argument in the package was
independently verified. Clean-architecture, SOLID, and DDD passes found no
blocking issues. `make build && make test && make lint` are green.

The substantive findings are in the **test suite**, not the production
change: the wiring test's core security assertion is nested inside a guard
that can silently skip it, and its positive assertion is coincidence-prone
(Medium). Secondary findings: `styles.Safe` test coverage has gaps against
its own contract (bidi isolates, BOM, boundary runes), the `pathDisplayNote`
doc comment grew verbose and duplicates `styles.Safe`'s own doc, the
security-scoped-out editable-seed boundary has an incomplete deferral
rationale, a vague parameter name, a redundant classifier branch, and a
missing glossary anchor. None block; most are hardening or documentation.

Tick exactly one checkbox per issue for the fix you want applied, then run
`af.task.review-apply 41` in a fresh session.

## Wiring test's core security assertion can be silently skipped

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Assert Behavior, Not Mock Mechanics"; a test proves nothing if the real assertion can be skipped.

In `TestPathDisplayNote_SanitisesControlRunes`
(`internal/tui/modals/fields_test.go:27-37`) the assertion that actually
guarantees safety — that the injected raw sequence `re\x1bpo` never appears
verbatim in the frame — is nested inside `if strings.ContainsRune(view,
'\x1b')`. That guard is true today only because the lipgloss theme emits
real ESC bytes in SGR colour codes. If a future themeless or SGR-free
render path is used, the guard goes false and the core security check
stops executing — silently, with the test still green.

Separately, the positive proof `strings.Contains(view, `\x1b`)` (the four
literal chars) is coincidence-prone: it passes as long as those four chars
appear _anywhere_ in the frame, and does not pin that they came from the
sanitised path.

```go
// fields_test.go:27-37 — positive check is loose; negative check is conditional
if !strings.Contains(view, `\x1b`) {          // 4 chars anywhere in frame → weak proof
    t.Errorf(...)
}
if strings.ContainsRune(view, '\x1b') {       // guard: only true because theme emits SGR ESC
    if strings.Contains(view, "re\x1bpo") {   // the REAL security assertion — skipped if guard false
        t.Errorf(...)
    }
}
```

Choose one:

- [x] Hoist the negative assertion out of the guard so it always runs, and
      strengthen the positive one to the full escaped substring:
      `if !strings.Contains(view, `re\x1bpo`) { t.Errorf(...) }` (proves
      `Safe` ran, in one line) plus an unconditional
      `if strings.Contains(view, "re\x1bpo") { t.Errorf(...) }` for the raw
      sequence. The SGR-ESC explanatory comment stays.
- [ ] Minimal fix: only hoist the raw-sequence check (`re\x1bpo`) out of the
      `ContainsRune` guard so it can never be skipped; leave the positive
      `\x1b` assertion as-is.
- [ ] Leave as-is — the probe confirms lipgloss SGR never emits the literal
      four chars and the guard is always true today; documented as an
      accepted, theme-coupled assumption.

## `styles.Safe` tests miss branches of its own contract

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Test One Behavior At A Time"; cover each rejection branch and the comparison edges.

`isTerminalSafeRune` (`internal/tui/styles/styles.go:290-315`) rejects more
than the table exercises. The acceptance criteria are met, but against the
function's real contract there are untested branches:

- **Bidi isolates U+2066–U+2069** (`styles.go:303`, second `||` clause) —
  the table tests only the bidi _override_ U+202E, not an isolate; distinct
  spoofing class, distinct code arm.
- **BOM U+FEFF** (`styles.go:307`) — `unicode.IsControl` is false for it, so
  only that explicit branch catches it; untested. Must be written as
  `"a﻿b"` (a mid-file source BOM is a compile error).
- **Boundary runes** — nothing pins `0x1f`→quoted / `0x20`→kept, or
  `0x7e`→kept / `0x7f`→quoted / `0x80`,`0x9f`→quoted / `0xa0`→kept. An
  off-by-one in the range comparisons would pass the current suite.

```go
// styles.go:303,307 — two rejection arms with zero direct coverage
if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) { return false } // isolate arm untested
if r == 0xfeff || (r >= 0x200b && r <= 0x200d) { return false }                    // BOM untested
```

Choose one:

- [x] Add table rows: bidi isolate `"a⁦b"` and BOM `"a﻿b"` to the
      hostile-runes suite (both assert `strconv.Quote(input)`).
- [ ] Add the above plus a boundary-edge set locking `0x1f`/`0x20`,
      `0x7e`/`0x7f`, `0x80`/`0x9f`/`0xa0` against quote/identity.
- [ ] Leave as-is — the stated acceptance criteria only require one C1, one
      bidi override, and one zero-width formatter, all present; the rest is
      optional hardening.

## `pathDisplayNote` doc comment is oversized and duplicates `styles.Safe`'s doc

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Comments explain intent/constraints; Functions stay small enough that purpose is obvious.

The doc comment is now ~17 lines for a 5-line function
(`internal/tui/modals/fields.go:40-56`). The task-0041 paragraph restates
the quote-whole-string strategy and the `\t`/`\n` residual that
`styles.Safe`'s own doc (`styles.go:262-273`) already explains, so the
reader gets the same tradeoff twice, and some of the older text narrates
history rather than the invariant.

```go
// fields.go:50-56 — restates styles.Safe's own documented strategy
// styles.Safe quotes the whole string on any hostile rune, so
// the user sees the escaped literal instead. `\t` and `\n` are preserved
// by design (Safe's allow-switch), an accepted residual — a legal
// newline in a path may render a multiline title but is not a hijack.
```

Choose one:

- [x] Tighten to the invariant and delegate the "how" to `styles.Safe`'s
      doc, e.g.: `// value is passed through styles.Safe so control / bidi /
    zero-width runes in a raw path cannot reach the frame; Safe preserves
    \t and \n (see its doc).`
- [ ] Keep the paragraph but drop the duplicated strategy sentence (the
      "quotes the whole string…" line) since `styles.Safe`'s doc owns it.
- [ ] Leave as-is — the security rationale is load-bearing and the package
      tolerates long explanatory comments elsewhere.

## Editable-seed deferral rationale omits the persisted-hostile-data angle

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Treat External Input As Untrusted"; make risky operations visible.

The task correctly scopes out editable `huh.Input` seed values, but the
stated reason ("pre-sanitising a bound editable value would mutate what the
user is typing") only covers the _typing_ phase. It does not address the
_initial display_ of persisted bytes: `edit_project.go:44-52` seeds
`pathInput` from `initial.Path`, sourced from `~/.agentfiles/projects.json`
(hand-editable / Adopt-populated external data). bubbles/textinput echoes
the seed verbatim, so a raw ESC persisted in a project path renders into
the frame the moment the Edit modal opens — before any keystroke. Small
exposure (needs a hostile `projects.json`), but it is a real display
boundary, narrower than the already-documented `\t`/`\n` residual.

```go
// edit_project.go — initial.Path is persisted, possibly-hostile external data
Path: initial.Path,                     // raw bytes -> textinput.SetValue -> rendered unsanitised
// ...
pathInput(&state.Path, "The path of the project"),
```

Choose one:

- [x] Extend `description.md` "Out of scope" to name the persisted-data
      angle explicitly, so the residual is a known deliberate limit rather
      than an untested gap (recommended — keeps scope, records the risk).
- [ ] File a follow-up task covering all editable seeds sourced from
      persisted manifests (Edit Project, Edit Profile, Adopt-populated
      forms) as one unit.
- [ ] Leave as-is — the current "Out of scope" wording is sufficient.

## `pathDisplayNote` parameter `value` is vague at the sanitisation boundary

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — don't hide the rule behind vague names like `data`/`value`; use the project's ubiquitous term.

The argument that the whole function exists to sanitise is named `value`
(`internal/tui/modals/fields.go:57`). The task's own vocabulary is "picked
path" — an external, untrusted filesystem string. A name matching that term
makes the untrusted-input nature legible at the call site without leaning on
the doc comment.

```go
func pathDisplayNote(value string, description string) *huh.Note {  // "value" hides that it's an untrusted path
```

Choose one:

- [x] Rename the parameter to `pickedPath` (or `path`) to match the
      pathselector vocabulary.
- [ ] Leave as-is — the thorough doc comment carries the meaning and the
      other constructors in `fields.go` also use `value`, so within-file
      consistency is a defensible reason to keep it.

## Redundant C0/DEL/C1 range checks subsumed by `unicode.IsControl`

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — keep code a maintainer can verify without reconstructing it.

In `isTerminalSafeRune` (`internal/tui/styles/styles.go:290-315`) the
explicit C0 (`r < 0x20`), DEL (`r == 0x7f`), and C1 (`0x80..0x9f`) branches
are fully subsumed by the trailing `unicode.IsControl(r)` (Unicode class Cc
is exactly those ranges), because `\n`/`\t` are already short-circuited by
the leading switch. Correct behaviour, but logically redundant. The bidi
and zero-width branches must stay — those are class Cf, not covered by
`IsControl`.

```go
if r < 0x20 || r == 0x7f { return false }   // subsumed by unicode.IsControl below
if r >= 0x80 && r <= 0x9f { return false }   // subsumed by unicode.IsControl below
// ...
if unicode.IsControl(r) { return false }     // already covers all of the above
```

Choose one:

- [ ] Leave as-is — the explicit ranges self-document which threat class
      each covers and cost nothing at runtime (recommended; matches the
      doc comment's enumerated rationale).
- [x] Drop the C0/DEL/C1 range checks and rely on the single
      `unicode.IsControl(r)` branch; keep the bidi and zero-width branches.

## No glossary anchor for the terminal-safe sanitiser

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — update the glossary when a durable term appears; keep one canonical name per concept.

"terminal-safe", "hostile rune", and `styles.Safe` are now load-bearing
vocabulary across code, comments, plan, changelog, and tests, and the
codebase carries two distinct boundary sanitisers — `styles.Safe`
(quote-whole-string, terminal frame) and `appapi.SanitizeSubject` (strip +
collapse, commit subject) — with no glossary entry distinguishing them. Not
a bug, and arguably presentation vocabulary rather than core
profile/asset/project domain vocabulary.

Choose one:

- [x] Add a short `docs/glossary.md` entry naming `styles.Safe` the
      canonical sanitiser for strings reaching a lipgloss frame, cross-
      referencing `SanitizeSubject` as the distinct commit-subject one.
- [ ] Leave as-is — this is presentation-layer vocabulary, not domain
      (profile/asset/project) vocabulary, so a glossary term is not required.
