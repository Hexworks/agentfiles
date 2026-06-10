# 0020 changes

Two additive component extensions consumed by upcoming screen tasks under the
0015 UI refactor:

1. A new `mnemonic.Set` type that registers `*Button` instances under a single
   collection and panics on duplicate mnemonic runes (case-insensitive). The
   set exposes `View` for joined rendering, `Match` for key routing, and
   `Buttons` for read-only iteration. Screens stop hand-rolling slices of
   buttons.
2. A new `treetable.WithValueColumns` option that injects N intermediate value
   columns between the `Name` column and the optional `Actions` column. Each
   `ValueColumn` carries a `Value func(*Node) string` callback rendered once
   per (row, render). This unblocks the Plan Project Screen
   (`Name | Status | Current Action | Actions`) without forking a one-off
   `plantable`.

No behavior change for existing call sites: zero value columns and an
unmodified `mnemonic.Button` API both round-trip through pre-existing tests.

## Decisions

- `Set.Add` panics rather than returning an error — **Why:** duplicate
  mnemonics on the same screen are programmer errors, matching the existing
  `mnemonic.New` constructor convention (`panic` on nil action, missing rune
  in label, etc.). Behavior stays consistent across the package.
- `Set.Match` returns the matched button (or nil) instead of triggering it —
  **Why:** routing and side effects belong to the host, mirroring how
  `Button.Matches` / `Button.Trigger` already split responsibility. Set stays
  passive.
- `ValueColumn` embeds `Column` rather than redeclaring `Title` and `Width` —
  **Why:** matches the API quoted verbatim in task 0020 and in the parent
  task 0015's Treetable section. Keeps callers consistent if `Column` grows
  later.
- No `Button.Mnemonic()` accessor added — **Why:** it already existed on
  `*Button` (button.go:112). `Set` uses it directly for uniqueness checks.

## Assumptions

- The test for "callback invoked once per render per row" measures invocations
  during a single `refreshRows()` pass — **Why:** the spec phrasing in the
  description maps to one pass; `refreshRows` is the function the existing
  treetable already calls on every state change.

## Other Notes

- No ADR added: both changes are additive extensions inside existing
  packages, with the design pre-approved in 0015's spec.
- No new guidelines: nothing new in the patterns; uses existing functional
  options, existing case-folding helpers, existing test conventions
  (stdlib `testing`, table-style fixtures).
- Tests use the `tea.KeyPressMsg{Code: r, Text: string(r)}` construction
  pattern already established in `modal_test.go`, `help_test.go`, and
  `treetable_test.go`.

## `internal/tui/components/mnemonic/set.go`

New file. Registers buttons, enforces uniqueness, exposes view/match/buttons.

```go
// before
// (file did not exist)
```

```go
// after — Set is a passive collection enforcing case-insensitive mnemonic uniqueness.
type Set struct{ buttons []*Button }

func NewSet() *Set { return &Set{} }

func (s *Set) Add(b *Button) {
    r := unicode.ToLower(b.Mnemonic())
    for _, existing := range s.buttons {
        if unicode.ToLower(existing.Mnemonic()) == r {
            panic(fmt.Sprintf("mnemonic: duplicate mnemonic %q registered for %q (already used by %q)",
                b.Mnemonic(), b.Label(), existing.Label()))
        }
    }
    s.buttons = append(s.buttons, b)
}

func (s *Set) View(sep string) string { /* join b.View() in order */ }
func (s *Set) Match(kp tea.KeyPressMsg) *Button { /* first b.Matches(kp) or nil */ }
func (s *Set) Buttons() []*Button { /* defensive copy */ }
```

## `internal/tui/components/treetable/treetable.go`

Added `ValueColumn`, `valueColumns` model field, `WithValueColumns` option,
and spliced rendering between `nameCol` and `actionsCol` in `tableColumns()`
and `refreshRows()`.

```go
// before
type Column struct { Title string; Width int }

type Model struct {
    nameCol    Column
    actionsCol Column
    /* ... */
}

func (m *Model) tableColumns() []table.Column {
    out := []table.Column{{Title: m.nameCol.Title, Width: m.nameCol.Width}}
    if m.actionsFn != nil {
        out = append(out, table.Column{Title: m.actionsCol.Title, Width: m.actionsCol.Width})
    }
    return out
}

// refreshRows row builder:
row := table.Row{name}
if m.actionsFn != nil { /* append actions cell */ }
```

```go
// after — name → value columns (in order) → optional actions, both header and rows.
type Column struct { Title string; Width int }

type ValueColumn struct {
    Column
    Value func(*Node) string
}

type Model struct {
    nameCol      Column
    valueColumns []ValueColumn
    actionsCol   Column
    /* ... */
}

func WithValueColumns(cols ...ValueColumn) Option {
    return func(m *Model) { m.valueColumns = cols }
}

func (m *Model) tableColumns() []table.Column {
    out := []table.Column{{Title: m.nameCol.Title, Width: m.nameCol.Width}}
    for _, vc := range m.valueColumns {
        out = append(out, table.Column{Title: vc.Title, Width: vc.Width})
    }
    if m.actionsFn != nil {
        out = append(out, table.Column{Title: m.actionsCol.Title, Width: m.actionsCol.Width})
    }
    return out
}

// refreshRows row builder:
row := table.Row{name}
for _, vc := range m.valueColumns {
    row = append(row, vc.Value(n))
}
if m.actionsFn != nil { /* unchanged actions cell */ }
```

## Tests

- `internal/tui/components/mnemonic/set_test.go` — 6 tests: duplicate
  same-case panic, duplicate case-insensitive panic, `Match` returns first
  matching button, unknown key returns nil, `View` preserves insertion order,
  `Buttons` returns defensive copy.
- `internal/tui/components/treetable/treetable_value_columns_test.go` — 5
  tests: zero value columns is transparent, four-column header with actions,
  cell values come from callbacks, `Value` invoked exactly once per row per
  render, actions cell remains cursor-only with value columns present.

## Verification

```
make build && make test && make lint
```

All green. Existing treetable and mnemonic tests pass unchanged; new tests
pass (`Go test: 6 passed` for mnemonic, `Go test: 10 passed` for treetable).
