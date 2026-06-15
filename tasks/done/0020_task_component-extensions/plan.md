# Plan — Task 0020: Component extensions

Cross-links:
- Task: [./description.md](./description.md)
- Parent task: `tasks/current/0015_task_refactor_ui/description.md` (sections "Mnemonic buttons / Safety" and "Treetable")
- Guidelines: `docs/guidelines/go.md`, `docs/guidelines/tui.md`, `docs/guidelines/charm.md`, `docs/guidelines/clean_architecture.md`, `docs/guidelines/clean_code.md`, `docs/guidelines/domain_model.md`, `docs/guidelines/solid.md`, `docs/guidelines/testing.md`

## Context

Parent refactor (task 0015) needs two component-level building blocks before screen tasks (e.g. 0029 Plan Project Screen) can land:

1. `mnemonic.Set` — runtime helper that enforces mnemonic-rune uniqueness across a screen's buttons, plus joins/matches them so screens stop hand-rolling slices of `*Button`.
2. `treetable.WithValueColumns` — extends the existing `treetable` to inject N value columns between `Name` and `Actions`, so the Plan Project Screen's `Name | Status | Current Action | Actions` layout reuses the widget instead of forking a `plantable`.

Both changes are additive. No existing call sites change behavior. Existing tests must keep passing.

## Part 1 — `mnemonic.Set`

### File

New: `internal/tui/components/mnemonic/set.go`
New: `internal/tui/components/mnemonic/set_test.go`

### Design

`Set` is a passive collection that wraps `*Button` values. Like the existing package, it is single-threaded and host-driven. No goroutines, no channels.

```go
type Set struct {
    buttons []*Button
}

func NewSet() *Set

// Add registers b. Panics with a descriptive message if any already-registered
// button shares the same mnemonic rune (case-insensitive). Programmer error.
func (s *Set) Add(b *Button)

// View renders every button via b.View() joined by sep, in insertion order.
func (s *Set) View(sep string) string

// Match returns the first button whose binding matches kp, or nil.
func (s *Set) Match(kp tea.KeyPressMsg) *Button

// Buttons returns the registered buttons in insertion order. Returned slice is
// a copy so callers can't mutate the internal store.
func (s *Set) Buttons() []*Button
```

Uniqueness uses `unicode.ToLower(rune)` to fold case — matches the existing case-handling in `button.go` (`containsRuneFold`, `View()` highlight). To read the rune from a `*Button`, expose a tiny package-private accessor or use a new exported `Mnemonic()` getter. Pick the smallest delta: add `func (b *Button) Mnemonic() rune` on `Button` — useful for tests anyway, and keeps `Set` from reaching into private state.

Joining uses `strings.Join` over `b.View()` results — same pattern as `main.go:105`. No new lipgloss call required.

`Match` iterates and calls the existing `Button.Matches(kp)`.

### Tests (stdlib `testing`, table-driven where useful)

`set_test.go`:

- `TestSetAddPanicsOnNilButton` — `Add(nil)` panics with `mnemonic: nil button`.
- `TestSetAddPanicsOnDuplicateMnemonicSameCase` — basic same-rune duplicate.
- `TestSetAddPanicsOnDuplicateMnemonicCaseInsensitive` — register `[Save]` with `'S'`, then `[send]` with `'s'`; expect panic.
- `TestSetAddDuplicatePanicMessageNamesConflictingLabels` — panic message contains both labels.
- `TestSetMatchReturnsFirstMatchingButton` — register two distinct buttons; build a `tea.KeyPressMsg` for each mnemonic; assert correct button returned.
- `TestSetMatchReturnsNilForUnknownKey` — unmatched key → nil.
- `TestSetMatchOnEmptySetReturnsNil` — empty Set returns nil for any key.
- `TestSetViewPreservesInsertionOrder` — add three buttons, assert each label appears in registration order in `View()`.
- `TestSetViewUsesConfiguredSeparator` — `WithSetStyles(Styles{Separator: " | "})` renders the configured separator between buttons.
- `TestSetViewOnEmptySetIsEmpty` — `View()` on empty Set returns the empty string.
- `TestSetButtonsReturnsCopy` — mutate returned slice, assert internal store unchanged.
- `TestSetButtonsOnEmptySetReturnsEmpty` — empty Set returns a zero-length slice.

Construct keys via the same path the example uses (`bubbles/v2/key.Binding` → simulated `tea.KeyPressMsg`). If there is no easy constructor, use the same fixture shape `Button.Matches` already accepts in `button.go`.

## Part 2 — `treetable.WithValueColumns`

### Files

Modified: `internal/tui/components/treetable/treetable.go`
New: `internal/tui/components/treetable/treetable_value_columns_test.go`

### Design

Match the spec verbatim — embed `Column`, not duplicate fields:

```go
type ValueColumn struct {
    Column
    Value func(*Node) string
}

func WithValueColumns(cols ...ValueColumn) Option {
    return func(m *Model) { m.valueColumns = cols }
}
```

Store on `Model` as `valueColumns []ValueColumn` (insert into the struct between `nameCol` and `actionsCol` to mirror render order — purely cosmetic).

#### Header (`tableColumns()`, treetable.go around line 162–168)

Build order: `[nameCol] → [valueColumns...] → [actionsCol if actionsFn != nil]`. Each `ValueColumn` contributes one `table.Column{Title: vc.Title, Width: vc.Width}`.

#### Rows (`refreshRows()`, treetable.go around line 180–202)

After `row := table.Row{name}`:

```go
for _, vc := range m.valueColumns {
    row = append(row, vc.Value(n))
}
```

Then the existing actions append runs unchanged.

`vc.Value(n)` is invoked exactly once per (row, render) pair — the existing `refreshRows()` already drives one pass per render trigger, so the spec's "called once per render per row" comes for free.

### Tests

New file `treetable_value_columns_test.go`. Reuse the helpers from `treetable_test.go` (tree fixture, model construction).

- `TestValueColumnsZeroIsTransparent` — model built with `WithValueColumns()` (no cols) renders identical headers/rows to a model built without the option.
- `TestValueColumnsHeaderHasFourColumnsWhenActionsEnabled` — `WithValueColumns(vc1, vc2)` + `WithActions(...)` → header length 4: Name, Status, Current Action, Actions. Titles and widths flow through.
- `TestValueColumnsCellsRenderFromCallbacks` — asserts the materialized cells via `Model.Rows()` equal the `Value(n)` outputs.
- `TestValueCallbackInvokedForEachRow` — counter inside `Value`; one `refreshRows` pass should fire the callback once per row.
- `TestActionsStayCursorOnlyWithValueColumns` — scan `Model.View()`: the rendered button label appears exactly once before and after `MoveDown`, proving non-cursor rows render an empty actions cell.
- `TestValueColumnsPanicOnNilValueCallback` — `WithValueColumns(ValueColumn{...})` with omitted `Value` panics at construction.
- `TestValueCellSanitizationNeutralizesControlCharacters` — callback returns `"x\ny\x1b]52;c;evil\x07z"`; rendered cell contains no `\n`, `\x1b`, `\x07`, and `\n` becomes a space.
- `TestValueCellSanitizationTruncatesToWidth` — long payload is truncated using ANSI-aware width to the column width.
- `TestPanelWidthStaysSquareWithValueColumnsAndTitle` — model with `WithValueColumns`, `WithTitle`, and `WithMnemonicButton`; every line of the rendered panel has identical `lipgloss.Width`.

## Out of scope (per description.md)

- Plan Project Screen (task 0029).
- Wiring `Set` into any existing screen.

## Step-by-step execution

1. Add `Mnemonic() rune` accessor on `*Button` in `button.go` (one line). No existing call sites; keeps `Set` from reaching into private fields.
2. Write `internal/tui/components/mnemonic/set.go` with `Set`, `NewSet`, `Add`, `View`, `Match`, `Buttons`.
3. Write `internal/tui/components/mnemonic/set_test.go`.
4. Add `ValueColumn` type and `valueColumns` field + `WithValueColumns` option in `treetable.go`.
5. Patch `tableColumns()` to splice value columns between name and actions.
6. Patch `refreshRows()` to append value-column cells after name and before actions.
7. Write `internal/tui/components/treetable/treetable_value_columns_test.go`.
8. Run `make build && make test && make lint`. Fix until green.
9. Set `description.md` `status: in-review`.
10. Write changelog under `docs/changelog/2026-06-10_0020-component-extensions.md`.

## ADRs / docs / guidelines

None. Both changes are additive within existing packages; the parent task's spec already captured the design choice (no separate `plantable`, runtime-helper + per-screen test layering). No new patterns warranting a guideline edit.

## Verification

```
make build && make test && make lint
```

Plus:
- New tests above must pass.
- `treetable_test.go` (existing) must keep passing — confirms zero behavior change when value columns aren't used.
