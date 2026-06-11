# Component extensions review

Task 0020 adds `mnemonic.Set` (a passive collection that enforces case-insensitive
mnemonic uniqueness at registration) and `treetable.WithValueColumns` (an option
that splices N value columns between `Name` and `Actions`). The changes are
additive, existing tests stay green, and the architectural fit is clean — the
DDD and Clean-Architecture passes returned no findings, and `make build && make
test` is green at HEAD.

The strongest signals from the parallel review:

- **Construction-time validation is missing for `ValueColumn.Value` and for `Set.Add(nil)`.** Four reviewers (Security, SOLID, Go, Testing) converged on the same hole: the package already panics-fast for other programmer errors (`mnemonic.New` does it for nil action, empty label, missing rune) but the new code skips this for the new contracts and surfaces opaque stdlib nil-deref panics from inside `refreshRows`. This is the single highest-priority finding.
- **`TestActionsStayCursorOnlyWithValueColumns` does not actually prove cursor-only behavior.** The test only asserts that `len(m.Buttons()) == 1` after a `MoveDown`. That assertion holds even if the implementation regressed to populating every row's actions cell with buttons, because `m.Buttons()` returns `m.currentBtns` which is rebuilt for the new cursor row regardless. The pre-existing `TestActionsScopedToCursor` uses a recorder closure that catches the real invariant — this test should mirror that pattern.
- **`Set.Buttons` shallow-copy doc is misleading** — the slice header is copied but the `*Button` elements are shared. Today there are no mutators on `*Button`, but the doc reads stronger than it is.
- **Doc/contract drift on `ValueColumn.Value`** — `"invoked once per (row, render)"` overstates the guarantee. The code re-invokes the callback on every `Update`, so a host that builds an expensive callback (DB lookup, hash recompute) will be surprised.
- **Tests reach through unexported `refreshRows` and `m.table.View`** — the new tests extend the pattern from `treetable_test.go`. Acceptable for parity, but worth deciding whether to formalize a testing seam.
- A handful of smaller polish items: `Model`'s column-kinds surface is widening (third splice point added to `tableColumns` and `refreshRows`); `Set.Add` package doc says "first render" but the panic fires at registration; `Set.View(sep)` makes separator a caller concern with no in-package default; panel + value-columns interaction has no test guard; plan/changelog reference a renamed test name; `slices.Clone`/panic message polish.

The TUI layering boundary is respected, no dependency direction is broken, and
the new types do not reach into private state. Below: 13 distinct findings,
each with a chosen-solution checklist.

## Nil `ValueColumn.Value` callback panics inside `refreshRows` with no construction-time check

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Return Actionable Errors"
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Liskov Substitution Principle ("Do not make a caller check which concrete implementation it received before it can use the value safely")
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Treat External Input As Untrusted"

`WithValueColumns` accepts the slice of `ValueColumn` verbatim. `refreshRows`
then calls `vc.Value(n)` unconditionally on every row. A caller who forgets
the `Value` field on one column (or builds columns in a loop and slips a
zero-valued entry through) does not see the failure at construction. It
surfaces deep inside the first render as a nil-function panic in
`refreshRows` with the stdlib message
`runtime error: invalid memory address or nil pointer dereference` and a
stack trace that points into treetable internals, not the caller's option
list.

This is exactly the failure mode `mnemonic.New` was written to avoid: it
panics with `mnemonic: nil action` / `mnemonic: empty label` /
`mnemonic: mnemonic rune not present in label` at construction time so the
panic identifies the contract that was violated. `WithActions` already
documents that "fn must be non-nil; passing nil disables the column", but
`WithValueColumns` has no equivalent enforcement. The two cases should be
symmetric.

```go
// internal/tui/components/treetable/treetable.go
func WithValueColumns(cols ...ValueColumn) Option {
    return func(m *Model) { m.valueColumns = cols } // accepts ValueColumn{Value: nil}
}

func (m *Model) refreshRows() {
    // ...
    for _, vc := range m.valueColumns {
        row = append(row, vc.Value(n)) // PANIC: nil-deref far from the misconfigured call site
    }
    // ...
}
```

- [x] Validate in `WithValueColumns`: panic with `"treetable: ValueColumn %q has nil Value callback"` (and ideally `Title != ""`), identifying the offending index. Add a `TestWithValueColumnsPanicsOnNilCallback` using `defer/recover`. Note that an empty string is a valid value in a column.
- [ ] Treat nil `Value` as "render empty string" instead — permissive semantics; document the choice and add a test pinning it.
- [ ] Leave as-is and rely on the stdlib panic.

## `Set.Add` accepts nil `*Button` with an opaque nil-pointer panic

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Treat External Input As Untrusted"
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Return Actionable Errors"

`Set.Add` documents itself as panicking on duplicate mnemonics — a deliberate
"programmer-error" contract mirroring `mnemonic.New`'s panics on nil action /
empty label / missing rune. But `Add` never checks `b != nil` before
dereferencing it. Passing a nil `*Button` panics in `b.Mnemonic()` with the
generic `runtime error: invalid memory address or nil pointer dereference`,
which is hard to attribute to this call site during development. The
package's existing style is to fail loudly with an actionable message
(`mnemonic: nil action`, `mnemonic: empty label`) at the registration
boundary — `Set.Add` should follow the same pattern.

```go
// internal/tui/components/mnemonic/set.go
func (s *Set) Add(b *Button) {
    r := unicode.ToLower(b.Mnemonic()) // PANIC: nil deref with no context if b == nil
    for _, existing := range s.buttons {
        if unicode.ToLower(existing.Mnemonic()) == r {
            panic(fmt.Sprintf("mnemonic: duplicate mnemonic %q ...", ...))
        }
    }
    s.buttons = append(s.buttons, b)
}
```

- [x] Add `if b == nil { panic("mnemonic: nil button") }` at the top of `Add`. Cover with `TestSetAddPanicsOnNilButton`.
- [ ] Leave as-is and accept the stdlib panic.

## `TestActionsStayCursorOnlyWithValueColumns` does not prove the cursor-only invariant

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Assert Behavior, Not Mock Mechanics" / "Test One Behavior At A Time"

The test only asserts `len(m.Buttons()) == 1` before and after `MoveDown(1)`.
That assertion holds whenever the cursor row has one button — it does NOT
prove that non-cursor rows render an empty actions cell. The whole point of
"cursor-only" behavior is that non-cursor row cells are blank. The test as
written would still pass even if the implementation regressed to populating
every row's actions cell with buttons.

Compare the pre-existing `TestActionsScopedToCursor` in `treetable_test.go`,
which uses an `actionsCalls` recorder to prove the callback fires for
exactly one row. The new test reduces to a duplicate of
`Buttons()`-after-move plumbing, which is already covered by
`TestSelectedNodeFollowsCursor`.

```go
// current test: passes even if every row renders buttons
if len(m.Buttons()) != 1 { // m.currentBtns is always set for the cursor row
    t.Fatalf(...)
}

// stricter — record actionsFn invocations:
var calls []string
fn := func(n *Node) []*mnemonic.Button {
    calls = append(calls, n.Label)
    return []*mnemonic.Button{mnemonic.New("Delete", 'D', noop)}
}
// ...refreshRows / Update...
if len(calls) != 1 { t.Fatalf("actionsFn called for non-cursor rows: %v", calls) }
```

- [ ] Rewrite to use an `actionsCalls` recorder mirroring `TestActionsScopedToCursor`, so the test fails if `actionsFn` runs for non-cursor rows.
- [x] Additionally scan `m.table.View()` for the button label and assert it appears exactly once.
- [ ] Leave as-is; the cursor-only behavior is already covered by `TestActionsScopedToCursor` without value columns.

## `Set.Buttons` defensive copy only protects the slice header, not the elements

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Data And Objects ("Hide internal structure when callers should not rely on it")
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Interface Segregation Principle
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Protect local state"

`Set.Buttons` correctly copies the slice header so callers cannot reorder,
truncate, or extend the internal `buttons` slice. But the copy is shallow:
both slices hold the same `*Button` pointers. The doc comment says
"mutating it does not affect the Set" — accurate for the slice header,
easy to misread as "the returned buttons cannot affect the Set". Today
there are no mutators on `*Button`, so the practical risk is small, but
future additions (`SetEnabled`, `SetLabel`, ...) would silently leak through.

```go
// internal/tui/components/mnemonic/set.go
func (s *Set) Buttons() []*Button {
    out := make([]*Button, len(s.buttons))
    copy(out, s.buttons) // copies pointers; future Button mutators would leak through
    return out
}
```

- [x] Tighten the godoc: "The returned slice header is a copy; reordering or replacing entries does not affect the Set. The pointed-at Buttons are shared — do not mutate them through the returned slice."
- [ ] Drop `Buttons()` entirely; expose `Each(func(*Button))` for help-bar iteration instead.
- [ ] Replace with `ButtonsView() []ButtonView` returning a read-only-accessor interface (label, mnemonic, binding only).
- [ ] Leave as-is; current behavior is fine for current consumers.

## `ValueColumn.Value` doc overstates "once per render"

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Functions ("Avoid surprising side effects") and Data And Objects ("Avoid hybrids that expose fields while also requiring callers to know hidden behavior")

The package doc and the `ValueColumn` doc both say "Value is invoked once
per (row, render)". That is an implementation detail of `refreshRows`, not
a contract the package can keep — `Update` calls `refreshRows` on every
message, so any keystroke re-invokes every callback. A host that reads the
comment and writes an expensive callback (DB lookup, hash recompute) will
be surprised when navigation or focus changes silently re-invoke it.
`TestValueCallbackInvokedOncePerRowPerRender` reinforces the misleading
promise by mirroring the current control flow.

```go
// ValueColumn doc — implies a stronger guarantee than the code provides
type ValueColumn struct {
    Column
    Value func(*Node) string // "invoked once per (row, render)"
}

// Every Update triggers another full pass:
m.table, cmd = m.table.Update(msg)
m.refreshRows() // every keystroke; every callback fires again
```

- [x] Rewrite the doc: "Value may be invoked on every render and on every Update; keep it cheap and pure." Rename or drop `TestValueCallbackInvokedOncePerRowPerRender`.
- [ ] Cache rendered cell text on the row and only re-invoke `Value` on explicit invalidation (`SetRoot`, a new `Invalidate()` API); then the existing doc is accurate.
- [ ] Leave as-is.

## Tests reach through unexported `refreshRows` and `m.table.View`

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Assert Behavior, Not Mock Mechanics" ("mock the code path that the test is supposed to verify" is a "Don't")
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Tests ("Test behavior, not implementation details")

Every new test in `treetable_value_columns_test.go` calls `m.refreshRows()`
directly and reads `m.table.View()` / `m.flat` / `m.currentBtns`. The public
driver is `Update(msg)` and `View()`; the inner table is unexported. By
poking the unexported method, the tests bind to an internal name. If
`refreshRows` is ever renamed or inlined the tests break for reasons
unrelated to behavior.

The pre-existing `treetable_test.go` already follows this pattern, so the
new file is not a regression — but it widens the coupling rather than
correcting it.

```go
// current — touches three unexported names
m.Focus()
m.refreshRows()
out := m.table.View()
for _, n := range m.flat { ... }
```

- [ ] Drive the new tests through `m.View()` and `m.Update(msg)` only.
- [x] Add a documented testing seam: expose `Refresh()` (or keep unexported and add a comment that tests may call it). Same for a `Rows() []table.Row` accessor.
- [ ] Accept the existing internal-coupling pattern for parity with `treetable_test.go`.

## `Model` column-kind surface is widening

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Single Responsibility & Open/Closed Principles

`Model` already knows about three column kinds (name, value, actions),
each with its own field, its own splice into `tableColumns()`, and its own
splice into `refreshRows()`. Each is hardcoded with positional rules: name
first, actions last, value columns in the middle. Adding the value column
required editing both render-shaping functions — the "edit stable
behavior" pattern OCP suggests avoiding when variation is real.

The next column kind (icon, checkbox, aggregate) will require another
field, another `With*` option, another splice in `tableColumns()`, another
splice in `refreshRows()`. Tractable now; trend is wrong.

```go
type Model struct {
    nameCol      Column          // kind 1
    valueColumns []ValueColumn   // kind 2 (new this task)
    actionsCol   Column          // kind 3
    actionsFn    ActionsFunc
    // ...
}
```

- [ ] Generalize to a single ordered `columns []columnSpec` slice where `columnSpec` carries `Title`, `Width`, and a `Render(*Node, isCursor bool) string` func. Name, value, and actions become three constructors over the same internal type.
- [x] Add a doc comment on `Model` declaring "exactly three column kinds: name, value, actions; new kinds require model edits" so the constraint is explicit, and defer generalization until a fourth kind appears.
- [ ] No action — three kinds is acceptable for the current scope.

## `refreshRows` mixes name, value, and actions cell assembly

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Functions ("Let each function do one coherent job at one level of abstraction")

`refreshRows` now owns four concerns at three abstraction levels: cursor
lookup, flat-index → prefixed tree line mapping, value cells via callbacks,
and the cursor-only actions cell (plus the side effect of storing
`m.currentBtns`). The hidden ordering between "iterate `valueColumns`" and
"then append actions" is what makes the column order correct, and it lives
inside the loop body rather than in a named helper.

```go
for i, n := range m.flat {
    name := ""
    if i < len(m.treeLines) {
        name = m.treeLines[i]
    }
    row := table.Row{name}
    for _, vc := range m.valueColumns {
        row = append(row, vc.Value(n))   // value cells
    }
    if m.actionsFn != nil {
        cell := ""
        if i == cursor {
            btns := m.actionsFn(n)
            m.currentBtns = btns          // hidden side effect on Model
            cell = renderButtons(btns)
        }
        row = append(row, cell)
    }
    rows[i] = row
}
```

- [x] Extract `buildRow(i int, n *Node, cursor int) table.Row` and keep `refreshRows` to the cursor bookkeeping + `SetRows` skeleton.
- [ ] Additionally hoist the `m.currentBtns = btns` assignment out of the per-row loop into a single explicit step after locating the cursor node.
- [ ] Leave as-is; function is still readable.

## `Set.View` separator is a caller concern with no in-package default

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Naming ("Make meaningful distinctions"); Data And Objects ("Prefer behavior on the package or type that owns the rule")

`Set.View(sep string)` is the only place in `mnemonic` where the join
separator is a caller concern. `Button.View()` is parameterless. Nothing
in the package guides hosts toward the conventional value; every consumer
will pick its own (`" "`, `" │ "`, `"  "`...) and drift apart over time.

```go
// Every host site looks like:
help := buttonSet.View(" ")     // why a space? where is that documented?
help := buttonSet.View(" │ ")   // legitimate variation or accidental drift?
```

- [ ] Drop the parameter; adopt an in-package default separator (`" "`). Add a `ViewSep(sep)` escape hatch only if a real consumer needs it.
- [x] Expose the separator on the `Styles` struct (alongside `Bracket` / `Label` / `Mnemonic`) so theming centralizes it where the other rendering knobs already live.
- [ ] Leave as-is; explicit-separator API is fine for two-or-three call sites.

## `Set.Add` package doc says "first render" but panic fires at registration

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Comments ("Do not repeat what the next line of code already says"; "Comment non-obvious invariants… compatibility requirements")

The doc comment on `Set` claims "the runtime panic catches it at first
render during development". `Add` actually panics at registration time —
the two are equivalent in trivial flows but diverge once a screen
registers buttons conditionally. A maintainer chasing a duplicate that
surfaces only after the user enters a particular state will be misled by
the "first render" framing.

```go
// "the runtime panic catches it at first render during development"
// — but the panic is here, at registration, not at View:
func (s *Set) Add(b *Button) {
    r := unicode.ToLower(b.Mnemonic())
    for _, existing := range s.buttons {
        if unicode.ToLower(existing.Mnemonic()) == r { panic(...) }
    }
    s.buttons = append(s.buttons, b)
}
```

- [x] Reword the package doc: "at registration time" instead of "at first render". Optionally also back `Set` with a `map[rune]*Button` keyed by the lowercased mnemonic to make the duplicate check O(1).
- [ ] Reword only; leave the data structure unchanged.
- [ ] Leave as-is.

## Panel + value-columns + title interaction is untested

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Tests ("Add or update tests when changing domain rules… or behavior"); Code Smells ("Fragility: one change breaks distant behavior")

`Model.View()` builds its panel width from `lipgloss.Width(body)`. `body`
is now wider when value columns are present, and `renderPanel` silently
clamps `pad` to zero if the title exceeds the body. With long captions
plus a focus mnemonic plus many value columns the top border can end up
shorter than the rows below — no test guards against that regression.
`TestValueColumnsCellsRenderFromCallbacks` reads `m.table.View()`, which
bypasses `renderPanel` entirely.

```go
// renderPanel clamps silently:
pad := bodyW - titleW
if pad < 0 {
    pad = 0     // title can exceed bodyW: top border shorter than rows below
}
```

- [x] Add a test that builds a model with `WithValueColumns`, `WithTitle`, and `WithMnemonicButton`, then asserts `View()` returns a frame where top, middle, and bottom lines all have equal `lipgloss.Width`.
- [ ] Factor the width math into a helper (`framedWidth(body, title) int`) and unit-test that directly.
- [ ] Leave as-is; panel rendering is unchanged by this task.

## Untrusted strings from `ValueColumn.Value` flow into row cells unsanitized

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Treat External Input As Untrusted"

`Value` callbacks typically compute cell text from `Node.Data`, which is
`any` payload originating in profile manifests or project state.
`refreshRows` forwards the returned string to `bubbles/table` as a row
cell with no normalization. Embedded `\n`, `\r`, `\t`, NUL bytes, or raw
ANSI escapes in the cell text will break column alignment, leak styling
outside the cell, or — worst case — emit terminal control sequences
(cursor moves, OSC clipboard writes) when rendered to a TTY. Same
property exists for `Node.Label` today, but the new `WithValueColumns`
widens the surface and is the obvious place to add a guard.

```go
for _, vc := range m.valueColumns {
    row = append(row, vc.Value(n)) // raw payload-derived text
    // "\x1b]52;c;<b64>\x07" or "\n" lands in the cell unchanged
}
```

- [x] Sanitize callback output before appending: strip control characters / ANSI CSI/OSC sequences, collapse newlines to spaces, truncate to column width. Add a test feeding `"x\ny\x1b]52;c;evil\x07"` and assert the cell is neutralized.
- [ ] Document the contract on `ValueColumn` ("Value must return a single-line, control-character-free string") and validate at runtime with a clear panic.
- [ ] Leave as-is; treat as a future concern when an actual untrusted payload reaches treetable.

## Plan / changelog reference a renamed test

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — naming consistency between plan, changelog, and implementation

Plan promises `TestSetAddPanicsOnDuplicateMnemonic` (line 60). Implementation
ships `TestSetAddPanicsOnDuplicateMnemonicCaseInsensitive`. The behaviors
are still covered (and the new name is clearer), but the rename is silent.

- [x] Update `plan.md` and the changelog to reflect the actual test name.
- [ ] Rename the test back to match the plan.
- [ ] Leave as-is; the rename is minor and the new name is more descriptive.

## Polish bundle

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md)

Three small idiom / polish items raised by the Go reviewer:

1. `Set.Buttons` uses `make`+`copy`; the Go-1.21+ idiom is `slices.Clone(s.buttons)`.
2. `Set.Add` panic message names the new button's rune+label and the
   conflicting button's label, but not the conflicting button's rune.
   Asymmetric diagnostics.
3. `ValueColumn` embeds `Column` so literals require
   `ValueColumn{Column: Column{Title: ..., Width: ...}, Value: ...}`. A
   flat struct would simplify call sites. Embedding is intentional only if
   future `Column` methods should be promoted onto `ValueColumn`.

```go
// 1. Buttons
return slices.Clone(s.buttons)

// 2. symmetric panic message
panic(fmt.Sprintf("mnemonic: duplicate mnemonic %q for %q collides with %q (mnemonic %q)",
    b.Mnemonic(), b.Label(), existing.Label(), existing.Mnemonic()))

// 3. flat ValueColumn
type ValueColumn struct {
    Title string
    Width int
    Value func(*Node) string
}
```

- [x] Apply all three: switch to `slices.Clone`, symmetric panic message, flat `ValueColumn`.
- [ ] Apply `slices.Clone` and panic-message polish only; keep `ValueColumn` embedded (document the method-promotion intent in a doc comment).
- [ ] Leave as-is on all three.
