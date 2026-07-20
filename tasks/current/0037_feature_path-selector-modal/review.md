# Path selector modal review

The 15-AC test mapping is complete and `make build && make test && make lint`
passes. The safety mechanics work on the _primary_ path (Enter → navigate →
`isUnderRoot`), but the **Select** action bypasses the same gate — a directory
symlink to `/etc` sitting inside the constraint can be handed to the caller as
`Result{Path: <in-root symlink>, IsDir: true}` today. Two supporting weaknesses
compound it: `EvalSymlinks` failures silently fall back to lexical paths, and
the constraint is validated once at construction and never re-checked. Beyond
security, the review flags a handful of hygiene issues (dead field, else after
return, magic numbers, button reallocation on toggle), a testable-boundary /
DIP question around `Options.normalize`, and gaps in the test suite for
branches the plan explicitly named.

Findings grouped roughly by severity — security first, then structural, then
hygiene, then documentation / testing gaps.

---

## Select on a symlink row bypasses the constraint gate

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Keep File Access Inside Intended Roots"

The `navigate` path calls `isUnderRoot` on every Enter, so a directory symlink
whose target sits outside `Constraint` is rejected with a "Cannot leave …"
notification. `onSelectCursor` does none of that — it takes `entry.Abs` (a
lexical `filepath.Join(current, name)` produced by `buildEntries`) and drops
it straight into `Result.Path`. Pressing `s` on `out@` where
`out → /etc` therefore hands the caller a "safe-looking" absolute path that
walks out of the constraint the moment the caller dereferences it. Same hole
applies to file symlinks in `ShowFiles=true` mode; `FollowSymlinks=false` does
not help because the row is still surfaced and still selectable.

```go
// internal/tui/modals/pathselector/pathselector.go:223
func (c *Content) onSelectCursor() tea.Cmd {
    e := c.cursorEntry()
    if !e.isSelectable() {
        return nil
    }
    c.state = modal.Confirmed
    c.value = Result{Path: e.Abs, IsDir: e.isDir()} // no isUnderRoot check
    return nil
}
```

Pick one:

- [ ] In `onSelectCursor`, pre-resolve `e.Abs` through `safety.resolveAbs(_, true)` and gate the result through `isUnderRoot(resolved, c.opts.constraint)`. On failure emit the same "Cannot leave …" notification and leave the modal `Active`. Store the resolved path in `Result.Path` so the caller sees the post-symlink target.
- [ ] Refuse selection of symlink rows outright (`entryDirSymlink` / `entryFileSymlink`) whenever `Constraint != ""` — simpler but slightly less useful.
- [x] Bake the check into a single `selectionResult(e entry) (Result, tea.Cmd)` helper called by both `onSelectCursor` and `onSelectCurrent`, so no future action path can forget it.

---

## `resolveAbs` and `isUnderRoot` silently fall back to lexical paths on `EvalSymlinks` error

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "reject path traversal after evaluation"

Both helpers treat any `EvalSymlinks` failure as "use the lexical path". A
dangling symlink, a permission-denied on a path component, or a race that
deletes the leaf between `Abs` and `EvalSymlinks` all collapse to a lexical
containment check. A symlink `<root>/dangling → /etc/passwd_missing` will
pass `isUnderRoot(<root>/dangling, <root>)` because `EvalSymlinks` errors and
we compare lexically. Combined with the previous finding, this is directly
exploitable via a swapped-in symlink between listing and select.

```go
// internal/tui/modals/pathselector/safety.go:25
resolvedPath, err := filepath.EvalSymlinks(path)
if err != nil {
    resolvedPath = path // silent fallback — a symlink escape survives
}
// safety.go:56 — resolveAbs same shape:
if resolved, err := filepath.EvalSymlinks(abs); err == nil {
    return filepath.Clean(resolved)
}
return abs
```

Pick one:

- [x] Treat `EvalSymlinks` failure as a containment failure: return `false` from `isUnderRoot`, propagate a typed error from `resolveAbs`, surface a notification, leave `current` untouched.
- [ ] Walk the path component-by-component: `EvalSymlinks(parent)` + `Lstat(leaf)`. Reject when the leaf is a symlink whose `Readlink` target does not resolve cleanly inside the root. More work, no false negatives on dangling symlinks.
- [ ] Accept the current fail-open behavior and rely solely on the runtime `isUnderRoot` check catching the escape — **not recommended**; documents the risk explicitly instead of fixing it.

---

## TOCTOU: constraint is validated once at construction and never re-checked

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "treat symlinks and existing unexpected files as safety-sensitive"

`Options.normalize` stats the constraint once, stores the post-`EvalSymlinks`
absolute path in `resolvedOptions.constraint`, and every later
`isUnderRoot(resolved, c.opts.constraint)` compares against that frozen
string. If the on-disk constraint is replaced by a symlink to an ancestor
between construction and navigation, containment is still checked against the
stale resolved path and passes. Inside `buildEntries` the read is also split:
`os.ReadDir(dir)` first, then `classify(d, abs)` re-stats each dirent — a
symlink swapped between the two calls is classified against the swapped
target.

```go
// internal/tui/modals/pathselector/entries.go:76
dirents, err := os.ReadDir(dir)          // read #1
// ...
kind := classify(d, abs)                 // read #2 — os.Stat inside classify
```

Pick one:

- [ ] Re-`EvalSymlinks(c.opts.constraint)` inside every `navigate` / `refresh` and abort with a "Cannot leave …" notification if the resolution changed since construction. Cheap, one syscall.
- [x] Anchor all directory reads to an `*os.Root` (Go 1.24+) opened once at construction, so filesystem swaps under the modal cannot redirect reads.
- [ ] Accept the residual TOCTOU risk given the modal is single-user and short-lived — document the assumption explicitly in `security.md`.

---

## `entryParent.Abs` is a lexical parent, not symlink-resolved

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "reject paths that escape the expected root after evaluation"

`buildEntries` sets the `..` row's `Abs` to `filepath.Dir(filepath.Clean(dir))`
without resolving symlinks. Production callers only reach `buildEntries` via
`navigate`, which always feeds it a `resolveAbs`-resolved path, so today the
lexical parent is safe. But the invariant lives in the caller, not the
function — `buildEntries` is package-exported and used by tests directly. If a
future refactor passes an unresolved `dir` (or a new caller lands), the `..`
row becomes a lexical escape route.

```go
// internal/tui/modals/pathselector/entries.go:120
if dir != opts.constraint {
    out = append(out, entry{
        Name: "..",
        Abs:  filepath.Dir(filepath.Clean(dir)), // lexical parent only
        Kind: entryParent,
    })
}
```

Pick one:

- [x] Compute the parent through `resolveAbs(filepath.Dir(dir), true)` so the row's `Abs` matches the modal's containment guarantee independent of caller discipline.
- [ ] Assert the precondition inside `buildEntries` (`dir` must equal `resolveAbs(dir, true)`) via a debug-only check, keeping the current cheap computation.
- [ ] Move the `entryParent` construction out of `buildEntries` into `refresh`, where the resolved path is already in hand — removes the invariant instead of documenting it.

---

## `Options.normalize` mixes pure validation with three filesystem probes

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — "I/O at the edges"
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — SRP

`normalize` performs input canonicalization (trim, abs, extension lowercase),
three syscalls (`EvalSymlinks` + `Stat` per input), and the constraint policy
(`isUnderRoot`) in one method returning three distinct `DomainError` shapes.
That mixes a value-object validator with I/O and makes the policy step
untestable without a real temp directory. The three error types also blur the
domain rule — `StartUnreadableError` and `ConstraintUnreadableError` are two
spellings of "a required path was not a readable directory", differing only
by the field they came from.

```go
// internal/tui/modals/pathselector/options.go:74
func (o Options) normalize() (resolvedOptions, errs.DomainError) {
    // ... filepath.Abs, EvalSymlinks, os.Stat, isUnderRoot ...
```

Pick one:

- [x] Split `normalize` into `Options.canonicalize()` (pure — trim/abs/lowercase, testable without a fs) and a package-private `probe(canon canonOptions) (resolvedOptions, DomainError)` that owns the syscalls. Keeps I/O at the edge.
- [ ] Introduce a small `fs` interface (`Stat` + `EvalSymlinks`) and inject it into `normalize`. Enables table-driven tests without disk churn.
- [ ] Leave as-is — modal is the edge, tests already use `t.TempDir()` — but at minimum collapse `StartUnreadableError` + `ConstraintUnreadableError` into `PathNotReadableError{Field, Path, Err}` so callers can branch on one type and messaging carries the field name.

---

## Safety helpers are duplicated across three packages

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — "Do not reuse code by copying it between packages. Copied code drifts."
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

`safety.isUnderRoot` / `isAncestor` are byte-identical copies of the private
helpers in `internal/app/service.go`, and `resolveAbs` overlaps significantly
with `internal/asset/files.go` `ResolveRelative`. The plan justifies the copy
on the "must not depend on `internal/app`" rule — which is correct, but does
not force _duplication_. `internal/utils` already sits at the bottom of the
import graph and is imported by both `app` and `asset`; promoting the helpers
there satisfies inward dependency direction _and_ eliminates drift. Note the
copies already differ subtly: `internal/app/service.go`'s `isUnderRoot`
carries an extra `regDir != "." && ...` compound clause that this copy does
not.

```go
// internal/tui/modals/pathselector/safety.go:8-15
// Duplicates the helper in internal/app/service.go on purpose: this
// package sits in the TUI layer and must not depend on the app package
func isAncestor(ancestor, descendant string) bool { ... }
```

Pick one:

- [x] Extract `IsUnderRoot` / `IsAncestor` / `ResolveAbs` into `internal/utils/fs.go`; replace the three call sites (`app/service.go`, `asset/files.go` where applicable, `pathselector/safety.go`) with imports. One safety fix lands in one place.
- [ ] Keep the duplicate but land a `safety_parity_test.go` that pins the modal's implementation against a shared golden table shared with `app` — accepts the copy in exchange for drift detection.
- [ ] Leave as-is and update the doc comment to name the _real_ reason (avoid cross-cutting churn) instead of the "dependencies point inward" one that only blocks `app`.

---

## Notification emission couples the leaf modal to the TUI notifications subsystem

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — Stable Dependencies

`emitNotify` inside `pathselector.go:356` constructs a
`notifications.NotificationMsg` directly. That fixes the modal's error-
reporting mechanism to one delivery detail (the toast subsystem) and makes
`pathselector` unusable in a hypothetical host that surfaces errors
differently. The modal already returns typed `DomainError` at construction —
runtime errors could equally travel out as a package-local message the shell
translates.

```go
// internal/tui/modals/pathselector/pathselector.go:264
if !isUnderRoot(resolved, c.opts.constraint) {
    return emitNotify(errs.SeverityError, "Cannot leave "+c.opts.constraint)
}
```

Pick one:

- [x] Emit a package-local `ConstraintViolationMsg{Constraint string}` and `ReadDirErrorMsg{Path string; Err error}`; let the modal shell in `internal/tui/` translate to `notifications.NotificationMsg`. Modal becomes host-agnostic.
- [ ] Accept the coupling — the modal is designed for this TUI, there is no second host, and the extra indirection is dead weight. Land only if a second host materializes.

---

## `else` after error return + double-wrapped `ReadDirError` in `New`

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — idiomatic error handling
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — "Re-wrapping a domain error in another domain error loses the original severity"

Two smells in five lines: (1) the `if err != nil { return … } else { … }`
shape is nonidiomatic — the rest of the codebase short-circuits and lets the
happy path fall through unindented; (2) `buildEntries` already returns a
typed `ReadDirError`, which `New` then wraps inside a `StartUnreadableError`.
That's a `DomainError` inside a `DomainError`, exactly what `errors.md`
warns against; `errs.Collect` has to walk two typed layers for one failure.

```go
// internal/tui/modals/pathselector/pathselector.go:81
if entries, entErr := buildEntries(resolved.startFolder, resolved, c.showHidden); entErr != nil {
    return nil, StartUnreadableError{Path: resolved.startFolder, Err: entErr}
} else {
    c.entries = entries
}
```

Pick one:

- [x] Flatten and stop double-wrapping: `entries, entErr := buildEntries(...)`; `if entErr != nil { return nil, entErr }`; `c.entries = entries`. `ReadDirError` already carries `Severity() = SeverityError` and satisfies `errs.DomainError`.
- [ ] Flatten but keep the wrap for context, using `fmt.Errorf("start folder: %w", entErr)` inside `StartUnreadableError.Err` — retains the outer type but shows the caller a single-layer message.

---

## `resolvedOptions.caption` is dead state

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

`normalize` trims and stores `Caption` into `resolvedOptions.caption`, but the
field is never read. The actual caption is applied one directory up in
`select_path.go` (`opts.Caption`). Two locations for the same value invite
drift.

```go
// internal/tui/modals/pathselector/options.go:62
caption             string
// options.go:76
caption:             strings.TrimSpace(o.Caption),
```

Pick one:

- [x] Drop the field and the assignment.
- [ ] Move the "default caption" fallback into `normalize` and actually consume the field from the wrapper (`select_path.go` reads `resolved.caption` back).

---

## `onToggleHidden` reallocates the button on each press and leaves it stale on error

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

Each `h` press builds a brand-new `mnemonic.Button`, then rebuilds the
`mnemonic.Set` to swap it in. Two independent problems: (1) allocating a
button per keypress is object-identity churn no reader expects; (2) the
label-swap sits _after_ the `refresh` short-circuit, so when
`refresh(current)` returns a non-nil error command the label update is
skipped — the button says "Show hidden" while state says "showHidden=true".

```go
// internal/tui/modals/pathselector/pathselector.go:243
func (c *Content) onToggleHidden() tea.Cmd {
    c.showHidden = !c.showHidden
    if cmd := c.refresh(c.current); cmd != nil {
        return cmd // label update below is skipped
    }
    c.hiddenBtn = mnemonic.New(hiddenButtonLabel(c.showHidden), 'h', ...)
    c.rebuildButtonSet()
    return nil
}
```

Pick one:

- [x] Update the label on the existing button (add `SetLabel` to `mnemonic.Button` if missing) and drop the reallocation + `rebuildButtonSet` entirely.
- [ ] Update the label unconditionally _before_ returning the refresh cmd, so an I/O error does not leave a stale UI label.
- [ ] Roll back the `showHidden` toggle when `refresh` errors, so both state and label stay consistent — most defensive, matches "refresh never side-effects on error" convention elsewhere.

---

## Trailing `return nil` after a total switch in `onEnter` silences future gaps

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

Every existing `entryKind` value is handled in the switch, but the trailing
`return nil` sits _after_ the switch. A future kind added without a
corresponding case falls through to the unreachable-by-design return and
becomes a silent no-op — the opposite of what the fence should do.

```go
// internal/tui/modals/pathselector/pathselector.go:205-219
func (c *Content) onEnter() tea.Cmd {
    e := c.cursorEntry()
    switch e.Kind {
    case entryHeader, entryEmpty, entryFile, entryFileSymlink: return nil
    case entryDirSymlink: ...
    case entryParent, entryDir: return c.navigate(e.Abs)
    }
    return nil // dead-in-intent, hides future gaps
}
```

Pick one:

- [x] Move the trailing return into a `default:` case with a comment naming the invariant ("new entry kinds must be added above").
- [ ] Add an exhaustive-switch lint (e.g. `exhaustive` from `golangci-lint`) if the project ever adopts it; delete the trailing return to lean on the linter.

---

## Magic numbers in `SetSize`

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

`nameWidth := width - actionsColWidth - 8` and `SetHeight(height - 4)` are two
bare integers with an approximate inline comment. The `8` is even called out
as a guess ("≈ borders + separators + padding"). Anyone changing the modal's
chrome has to reverse-engineer both constants.

```go
// internal/tui/modals/pathselector/pathselector.go:166
nameWidth := width - actionsColWidth - 8 // 8 ≈ borders + separators + padding
// pathselector.go:170
c.tree.SetHeight(height - 4)
```

Pick one:

- [ ] Introduce `treeChromeWidth = 8` and `treeChromeHeight = 4` constants next to `actionsColWidth`, with a comment breaking down the summands (border, separator, padding, button row height).
- [x] Ask `treetable` / `modal` to expose their true chrome dimensions and drop the guesswork.

---

## `cursorEntry` silently returns `entryEmpty` on an out-of-range cursor

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

The out-of-range branch is intended as "should never happen" (the treetable
cursor is bounded by row count), but returning `entryEmpty` masks a genuine
off-by-one — `entryEmpty.isSelectable() == false`, so a bug becomes an
invisible no-op instead of a crash.

```go
// internal/tui/modals/pathselector/pathselector.go:189
func (c *Content) cursorEntry() entry {
    i := c.tree.Cursor()
    if i <= 0 { return entry{Kind: entryHeader, ...} }
    if i-1 >= len(c.entries) { return entry{Kind: entryEmpty} } // silent misclassify
    return c.entries[i-1]
}
```

Pick one:

- [x] Panic with a message naming the invariant (`treetable cursor exceeds row count`). Bug-finding beats silent no-op.
- [ ] Add a comment explaining that transient resize/refresh windows justify the fallback, plus a test that covers the reachable case; keep the current shape.

---

## `firstNonEmpty` misleads readers at the callsite

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

The helper name says "first non-empty of a variadic list" but the callsite
always has exactly two arguments and the domain intent is "prefer the
user-supplied form for the error message, fall back to the resolved path".
Same signal, wrong name.

```go
// internal/tui/modals/pathselector/options.go:127
return resolvedOptions{}, StartUnreadableError{Path: firstNonEmpty(startInput, start), Err: statErr}
// options.go:136
Start:      firstNonEmpty(startInput, out.startFolder),
```

Pick one:

- [x] Rename to `userFacingPath(input, resolved string)` and drop the variadic — signature matches intent.
- [ ] Inline as a two-liner at each callsite and delete the helper.

---

## Ubiquitous language drift: `Constraint` vs `Constraint Root`

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Do not let UI labels, docs, and code drift into different meanings"

The glossary defines the canonical term as **Constraint Root**, but code,
doc comments, and the user-facing notification uniformly say "constraint".
The user only ever sees the notification text; that is where the drift lands.

```go
// internal/tui/modals/pathselector/pathselector.go:265
return emitNotify(errs.SeverityError, "Cannot leave "+c.opts.constraint)
```

Pick one:

- [x] Rename `Options.Constraint` → `Options.ConstraintRoot` and update every doc comment / notification string to match the glossary. Widest churn but zero drift.
- [ ] Keep field name (`Constraint` is fine for ergonomics), but change the notification to `"Cannot leave constraint root <path>"` so the user-facing surface matches the glossary.
- [ ] Rename the glossary entry to `Constraint` — reverses the direction; only defensible if "root" adds no meaning.

---

## `entry.isDir()` conflates "row is a directory" with "selecting yields IsDir=true"

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Put Domain Rules In Domain Code"

`isDir()` returns true for `entryHeader` and `entryParent` in addition to
`entryDir`/`entryDirSymlink`. `entryHeader` is a synthetic UI row, not a
directory — the method encodes a _selection_ rule ("this row produces
`Result{IsDir:true}`"), not a filesystem fact. A future reader looking at
`entry.isDir()` in isolation gets the wrong mental model.

```go
// internal/tui/modals/pathselector/entries.go:55
func (e entry) isDir() bool {
    switch e.Kind {
    case entryHeader, entryParent, entryDir, entryDirSymlink:
        return true
```

Pick one:

- [x] Rename to `selectsAsDir()` — matches the actual rule and the callsite (`onSelectCursor` uses it to set `Result.IsDir`).
- [ ] Split into `isFilesystemDir()` (true only for real directories) and `selectionIsDir()` (adds header/parent). Callsites use the one that matches their intent.

---

## `moveCursorTo` test helper hides advance-by-one regressions

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)

The helper loops `len(c.entries) + 1` times pressing `j`, then falls back to
a repeat-check after `t.Fatalf` — which is unreachable. Two problems: (1) the
`+1` slack silently absorbs a regression where the tree cursor collapses two
rows into one keypress; (2) the post-`t.Fatalf` branch is dead code.

```go
// internal/tui/modals/pathselector/pathselector_test.go:329
for range len(c.entries) + 1 { ... _, _ = c.Update(keyPress("j", 'j')) }
if c.cursorEntry().Name == want { return } // dead after t.Fatalf
t.Fatalf(...)
```

Pick one:

- [x] Loop exactly `len(c.entries)` times, fail with a diagnostic (`row %q not reachable after %d presses`) if not found. Boring helper, exact promise.
- [ ] Add a dedicated `TestCursorAdvancesOneRowPerJ` unit test asserting the invariant the loop currently hides, keep the fallback for other reasons.

---

## Missing coverage of documented code paths

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)

Several branches described in the plan or changelog have no test:

- `classify` returning `entryFileSymlink` when `os.Stat` fails (dangling symlink) — `entries.go:141`.
- `[Select current]` (`c`) on an empty folder resolving with the empty folder's path.
- Header-row `Enter` and header-row `s` no-ops (asserted only indirectly via mnemonic uniqueness).
- `Constraint == ""` at the modal level — `isUnderRoot` is unit-tested, but no `Content`-level test proves unconstrained browsing works.
- `resolveAbs(follow=true)` when `EvalSymlinks` fails on a missing suffix — `safety.go:56`. Plan named this test explicitly (`TestSafetyResolveHandlesMissingSuffix`) and it did not land.
- `hiddenBtn` label swap on toggle — `TestHiddenToggle` asserts entry visibility only.
- `refresh` I/O error path — `TestReadDirDeniedNotifies` covers Enter-then-fail; the vanishing-current-folder case is not covered.

Pick one:

- [x] Land the missing tests as one file (`gaps_test.go`) covering: dangling symlink classification, empty-folder Select-current, header-row Enter/select, `Constraint == ""` navigation, `resolveAbs` fallback, hidden-button label swap.
- [ ] Land only the security-adjacent subset (dangling symlink + `resolveAbs` fallback + unconstrained navigation) and defer the rest — smaller diff, still closes the safety-critical gaps.
- [ ] Accept the current coverage — every AC is mapped and the missing paths are edge cases; document that in the changelog and move on.

---

## `TestConstraintRootUpwardNoop` proves less than its name claims

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)

The test's real assertion is "`entryParent` is absent when browsing at the
constraint root" — which is what AC #1 requires. The follow-up "send arrow-
up and assert no notification" tests the treetable's built-in cursor
clamping, not the modal's constraint logic. If a future refactor reintroduced
`entryParent` at the constraint root, this test would still pass while AC #1
broke.

```go
// internal/tui/modals/pathselector/pathselector_test.go:23
for _, e := range c.entries {
    if e.Kind == entryParent { t.Fatalf(...) } // real AC #1 check
}
// arrow-up check below only tests treetable clamping
```

Pick one:

- [x] Drop the arrow-up sanity and replace it with an explicit `c.navigate(filepath.Dir(root))` that must emit a "Cannot leave …" notification — tightens the security guarantee.
- [ ] Keep the current shape but rename the arrow-up phase to `_treetable_clamp` inside the test body so intent is documented.

---

## `t.Parallel()` absent across every test

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)

Every test uses `t.TempDir()` (isolation-safe) but none marks `t.Parallel()`.
CI wall-clock is longer than necessary; more importantly, "serial by default"
means any future ordering dependency will surface as a real bug rather than
being caught by parallel-safe conventions from day one.

Pick one:

- [x] Add `t.Parallel()` to every test in the package (including subtests). All use `t.TempDir()` and none mutate global state — safe.
- [ ] Add it only to the leaf `t.Run(...)` subtests; keep the outer functions serial to preserve any implicit ordering expectations.

---

**Reviewer**: read this file, tick exactly one checkbox per issue for the
solution you want applied. When ready, run
`af.task.review-apply 0037` in a fresh session.
