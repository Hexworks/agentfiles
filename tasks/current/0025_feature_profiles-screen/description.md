---
id: 0025
type: feature
status: pending
depends_on: 0021, 0022, 0023, 0024
---

# Profiles screen

Implements **Profiles Screen** from
`0015_task_refactor_ui/description.md`. This is the first screen behind the
Welcome menu's "Profiles" entry.

## Layout

```
╭──────────╮
│ Profiles │
╰──────────╯

┌────────────────────────────────────────────────────────────────────────────────────┐
│ ID          Name        Path                            Actions                    │
│────────────────────────────────────────────────────────────────────────────────────│
│ another     Another     /Users/addamsson/af/another                                │
│ test        Test        /Users/addamsson/af/profiles/   [Edit] [Delete]            │ <-- selected row
│ ...                                                                                │
└────────────────────────────────────────────────────────────────────────────────────┘
 [Create New Profile] [Register Profile]

{{ notification area }}

↑/k up • ↓/j down • n notifications • s settings • q quit
```

## Behavior

### Data load

- On `Init`, run the `LoadProfiles` action (task 0019). Cache the result for
  the screen's lifetime — re-fetch only after a mutation completes.

### Table

- `bubbles/table` with fixed height computed from the available terminal
  size (heading, button row, notification area, status bar are fixed).
- Cursor row selection. Selected row renders `[Edit]` and `[Delete]` in the
  Actions column.

### Row actions (mnemonic buttons)

- `e` → push the Edit Profile Screen (task 0026; until landed, a stub OK)
  with the selected profile's `id` as parameter.
- `d` → **two-step** delete:
    1. Open Confirmation Modal: `"Are you sure you want to delete profile {{name}}?"`
       - "No" → abort, no further prompts.
       - "Yes" → step 2.
    2. Open Confirmation Modal: `"Also delete profile folder on disk?"`
       - "No" → call `DeleteProfile` action with `KeepFolders`.
       - "Yes" → call `DeleteProfile` action with `DeleteFolders`.
    Outcome routed through the notification helper from task 0019.

Use `mnemonic.Set` (task 0020) on the selected row so duplicate mnemonics
panic at render time.

### Screen-level mnemonic buttons (always visible)

- `c` → open **Create Profile Modal** (task 0022). On confirm, invoke
  `CreateProfile` action with the typed input.
- `r` → open **Register Profile Modal** (task 0022). On confirm, invoke
  `RegisterProfile` action.

Per the parent task's status-bar rule, these screen-level buttons are NOT
repeated in the status bar.

## Status bar

Global keys plus, when a row is selected, the row mnemonics (`e edit`,
`d delete`). Screen-level `c`/`r` excluded.

## Tests

Per the parent task's safety rule, every screen ships a `*_test.go` that
walks the screen through each focus/selection state and asserts no two
visible mnemonic buttons share a key:

- No-selection state: only `[Create New Profile]` + `[Register Profile]`
  visible; mnemonics `c`, `r` unique.
- Selection state: row buttons + screen buttons visible; mnemonics `e`,
  `d`, `c`, `r` unique.

Plus:

- Two-step delete: confirming No on step 1 makes no service call.
- Two-step delete: confirming Yes/No invokes `DeleteProfile` with
  `KeepFolders`; Yes/Yes invokes it with `DeleteFolders`.

## Out of scope

- Edit Profile Screen body (task 0026).

## Verification

```
make build && make test && make lint
./bin/af   # Welcome → Profiles → Create/Register/Edit/Delete flows
```
