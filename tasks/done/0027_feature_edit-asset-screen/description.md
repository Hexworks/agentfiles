---
id: 0027
type: feature
status: done
topics: go, tui, asset
depends_on: 0021, 0022
---

# Edit Asset screen

Implements **Edit Asset Screen** from
`0015_task_refactor_ui/description.md`. Two columns: a file treetable on
the left, a Summary + Customize form group on the right. Most complex
single screen in the breakdown.

## Parameters

- `id`: the asset's id, passed when the screen is pushed.

## Data load

On `Init`, run the `LoadAsset(id)` action (task 0019).

## Left column — Files (50% width, 90% height)

`internal/tui/components/treetable` (already in place) showing the asset's
directory.

- `[1]` (`ctrl+1`) focuses the treetable.

Row actions (selected + focused row):

- `o` `[Open]` — **leaf nodes only** (files, not directories). Opens the
  file via `tui/editor.Open(path)`. After the editor returns
  (`editor.FinishedMsg`), call `UpdateAsset` action with the current
  in-memory `*asset.Asset`.
- `d` `[Delete]` — Confirmation Modal → on Yes: delete the physical file,
  remove it from the `Asset` object, call `UpdateAsset` action.

Below the treetable: screen-level mnemonic button:

- `a` `[Add]` (only when files treetable is focused) → open **Create File
  Modal** (task 0022). On confirm: create the physical file at the given
  path (relative to the asset folder), add it to the `Asset` object, call
  `UpdateAsset` action.

## Right column — Summary + Customize

**Summary** (50% width, 30% height; read-only):

- `Name` — `{{asset.Name}}`
- `Type` — `{{asset.Type}}`

**Customize** (50% width, 60% height; editable form group):

- `Description` — multi-line text input.
- `Tags` — comma-separated text input. List ↔ string conversion uses `,`
  as separator with whitespace stripped (per parent description's note:
  `["foo", "bar"]` ↔ `"foo, bar"`).
- `CompatibleAgents` — multi-select from `codex`, `claude-code`, `cursor`,
  `opencode`; empty = all.
- `ExclusiveGroup` — single-line text input.

- `[2]` (`ctrl+2`) focuses the `Description` field.

Whenever focus moves away from an input field (on `Blur()`), the in-memory
`*asset.Asset` is updated.

## Bottom-right buttons

- `e` `[Save]` → call `UpdateAsset` action with the in-memory asset.
- `b` `[Back]` → navigate to Profiles Screen. **Must ask for confirmation
  if there are unsaved changes** (compare current in-memory asset against
  the snapshot taken on Init).

## Sizing

Heading + buttons + notification area + status bar fixed; both columns sized
from the remaining real estate.

## Status bar

Global keys + dynamic row mnemonics. Screen-level buttons excluded; per the
parent description's example, `e save` and `b back` appear in the bar — keep
them (they're tied to focus state, not to the screen button row).

## Tests

Per the safety rule, walk every focus + selection state and assert mnemonic
uniqueness. Candidate alphabet: `{1, 2, o, d, a, e, b}`.

Plus:

- `[Open]` button is rendered only on leaf nodes — not on directory nodes.
- Editing a file → `UpdateAsset` is called after `editor.FinishedMsg`.
- `Tags` round-trip: editing `"foo, bar"` round-trips to `["foo", "bar"]`.
- Back-with-unsaved-changes opens a Confirmation Modal; Yes pops, No
  cancels.

## Out of scope

- The `UpdateAsset` hash-refresh behavior — already covered in task 0018.

## Verification

```
make build && make test && make lint
./bin/af   # Edit Profile → Edit Asset → Open, Edit, Save, Back flows
```

## Plan

[plan.md](./plan.md)
