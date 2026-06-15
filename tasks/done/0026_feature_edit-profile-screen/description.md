---
id: 0026
type: feature
status: done
topics: go, tui
depends_on: 0021, 0022
---

# Edit Profile screen

## Plan

See [plan.md](./plan.md).

Implements **Edit Profile Screen** from
`0015_task_refactor_ui/description.md`. Two tables (Assets, Projects) with
focus mnemonics, row context actions, and screen-level Create / Register /
Back buttons.

## Parameters

- `id`: the profile's id, passed when the screen is pushed.

## Data load

On `Init`, run the `LoadProfile(id)` action (task 0019).

## Focus model

Two `bubbles/table` widgets registered with a `focus.Handler`
(`internal/tui/components/focus`):

- `tab` / `shift+tab` cycle focus between the Assets and Projects
  tables. There are no panel mnemonic buttons; the focused panel is
  obvious from its accent-colored rounded border (unfocused panels use
  the muted grey border).

## Assets table

Columns: `Id`, `Name`, `Type`, `Actions`.

Row actions (selected + focused row only):

- `e` → push **Edit Asset Screen** (task 0027) with the asset id.
- `d` → open Confirmation Modal; on Yes, call `DeleteAsset` action.

Below the table, screen-level mnemonic button:

- `c` `[Create Asset]` → open **Create Asset Modal** (task 0022). On
  confirm, call `CreateAsset` action with the returned `asset.Manifest`.

## Projects table

Columns: `Id`, `Name`, `Path`, `Actions`.

Row actions (selected + focused row only):

- `e` → open **Edit Project Modal** (task 0022) prefilled with the selected
  project. On confirm, call `UpdateProject` action.
- `a` → push **Select Project Assets Screen** (task 0028) with the project
  id.
- `p` → push **Plan Project Screen** (task 0029) with the project id.
- `d` → open Confirmation Modal; on Yes, call `DeleteProject` action.

Below the table:

- `r` `[Register Project]` (left) → open **Register Project Modal**. On
  confirm, call `RegisterProject` action.
- `b` `[Back]` (right) → pop to Profiles Screen.

## Sizing

Each table renders at its natural width (per-column max of header
width and widest cell content) and natural height (one header row plus
N data rows). Both panels share a width: the wider panel's natural
width wins and the narrower one grows its elastic column (Name for
Assets, Path for Projects) to match. The heading, button row, and
status bar sit above/below at their own natural heights; nothing is
padded to fill the remaining viewport.

## Status bar

Global keys + the selected-row mnemonics of the focused table. Screen-level
`c` / `r` / `b` excluded per the parent task's status-bar rule. Per the
parent description's bottom-bar example, the Settings/Profiles screen
shows `b back` — Edit Profile's `b back` follows the same exception
(include it).

## Tests

Per the safety rule:

- Each focus + row-selection combination is walked: assert no two visible
  mnemonic buttons share a key. The candidate alphabet to test in
  combination is `{c, r, b, e, d, a, p}`.

Plus:

- Tab cycle order matches expected order.
- Confirmed Delete Project does not delete project files (only metadata —
  uses `DeleteProject` from task 0018).
- Pre-load `Body()` followed by a profile load and a second `Body()`
  recovers the cursor and shows every row + the action cell on the
  cursor row. Guards against the bubbles `SetRows(nil)` cursor=-1
  sticky bug.

## Out of scope

- Edit Asset Screen body (task 0027).
- Select Project Assets Screen body (task 0028).
- Plan Project Screen body (task 0029).

## Verification

```
make build && make test && make lint
./bin/af   # Profiles → Edit Profile → all row/screen actions reachable
```
