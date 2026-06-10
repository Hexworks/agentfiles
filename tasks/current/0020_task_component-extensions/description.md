---
id: 0020
type: task
status: in-review
topics: go, tui, charm
depends_on: 0015
---

# Component extensions: mnemonic.Set + treetable value columns

This task adds two component features the new screens depend on. Both extend
existing packages — no new directories. See parent task
`0015_task_refactor_ui/description.md`, sections **Mnemonic buttons /
Safety** and **Treetable**.

## Part 1 — `mnemonic.Set` registry helper

### Why

Per-screen unit tests guarantee uniqueness in CI, but a runtime helper
catches duplicate mnemonics at first render of any focus / selection state
during development. The parent task spells out both layers.

### API

```go
// internal/tui/components/mnemonic/set.go

// Set is a collection of Buttons that enforces mnemonic uniqueness.
// Screens register buttons through a Set instead of constructing them
// ad-hoc; the host then queries the Set for rendering and key routing.
type Set struct { /* … */ }

func NewSet() *Set

// Add registers b. Panics if another button with the same mnemonic rune
// (case-insensitive) is already registered. Programmer error.
func (s *Set) Add(b *Button)

// View renders every button joined by sep. Order = insertion order.
func (s *Set) View(sep string) string

// Match returns the first button whose binding matches kp, or nil.
func (s *Set) Match(kp tea.KeyPressMsg) *Button

// Buttons returns the registered buttons in insertion order (read-only
// slice copy).
func (s *Set) Buttons() []*Button
```

### Tests

- Adding a button with a duplicate mnemonic panics.
- Mnemonic match is case-insensitive (a `[Save]` registered with `'S'`
  collides with another button registered with `'s'`).
- `Match` returns the right button; non-matching key returns nil.
- `View` order = insertion order.

## Part 2 — `treetable.WithValueColumns`

### Why

The Plan Project Screen needs four columns:
`Name | Status | Current Action | Actions`. Forking a one-off `plantable`
widget would waste the existing treetable. Instead, extend treetable with
N injected value columns between `Name` and the optional `Actions` column.

### API

```go
// internal/tui/components/treetable/treetable.go (extension)

type ValueColumn struct {
    Column
    Value func(*Node) string
}

func WithValueColumns(cols ...ValueColumn) Option
```

`Value` is called once per render per row and returns the cell text. Render
order: `Name` → injected value columns (in order) → `Actions`. The `Actions`
column behavior (cursor-only render, mnemonic routing) is unchanged.

### Tests

- New `treetable_value_columns_test.go`:
    - N=0 → identical output to current behavior.
    - N=2 → header has 4 columns when actions are enabled; cell values come
      from the `Value` callbacks.
    - `Value` is called once per row per render (track via a counter in the
      callback).
    - Cursor-row actions still render only on the cursor row when value
      columns are present.

## Out of scope

- Plan Project Screen (task 0029).
- Any consumer of `mnemonic.Set` (later screen tasks).

## Verification

```
make build && make test && make lint
```

## Plan

[plan.md](./plan.md)
