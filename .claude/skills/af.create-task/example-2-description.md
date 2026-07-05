---
id: 0029
type: feature
status: done
topics: go, tui, project, render
depends_on: 0017, 0020, 0021
---

# Plan Project screen

Implements **Plan Project Screen** from
`0015_task_refactor_ui/description.md`. Single treetable with four columns;
per-row toggle for drift/unknown rows; `[Apply]` + `[Back]` screen buttons.

## Parameters

- `id`: the project's id, passed when the screen is pushed.

## Data load

On `Init`, run the `PlanProject(id)` action (task 0019), which wraps
`app.Service.Plan` and ultimately `sync.Plan` (which now classifies
`ChangeUnknown` correctly per task 0017).

## Treetable

Uses `treetable.WithValueColumns` (task 0020) to inject two value columns
between `Name` and `Actions`:

| Column         | Origin                                                         |
| -------------- | -------------------------------------------------------------- |
| Name           | path of file / directory                                       |
| Status         | `FileChange.Kind` (only on files, not directories)             |
| Current Action | currently chosen `Resolution` (only on files, not directories) |
| Actions        | one mnemonic button — the **other** option                     |

### Status display

- `+ add`, `~ update`, `- delete` for `ChangeCreate` / `ChangeUpdate` /
  `ChangeDelete` (no user action).
- `* drift` for `ChangeDrift` (needs user resolution).
- `? unknown` for `ChangeUnknown` (needs user resolution).

### Actions column — toggle button (cursor + focused row only)

| Status                   | Current Action | Button rendered | Mnemonic |
| ------------------------ | -------------- | --------------- | -------- |
| drift                    | Keep           | `[Overwrite]`   | `o`      |
| drift                    | Overwrite      | `[Keep]`        | `k`      |
| unknown                  | Keep           | `[Delete]`      | `d`      |
| unknown                  | Delete         | `[Keep]`        | `k`      |
| create / update / delete | — (auto)       | (no button)     | —        |

Defaults: `Keep` for both `drift` and `unknown`.

Pressing the button swaps `Current Action` and re-renders the row.

## Internal state

A `map[string]sync.Resolution` keyed by path, holding non-default
selections. Default-valued entries (`ResolveKeep`) need not be stored;
they're inferred at submission time.

## Screen-level buttons (below the table, right-aligned)

- `[Apply]` (mnemonic `a`) → build `[]sync.FileResolution` from the state
  map (entries only for paths the user toggled off-default), call
  `SyncProject` action.
- `[Back]` (mnemonic `b`) → pop to the previous screen.

## Sizing

Heading + button row + notification area + status bar fixed; the treetable
fills the rest.

## Status bar

Global keys + the dynamic toggle mnemonic for the selected row (`o`, `k`,
or `d`). Screen-level `a` and `b` excluded.

## Tests

Per the safety rule, walk every selection state (cursor on directory,
cursor on each `ChangeKind` row) and assert mnemonic uniqueness. Candidate
alphabet: `{o, k, d, a, b}`.

Plus:

- For each row state in the table above, the rendered button label +
  mnemonic match the expected entry.
- Toggling a drift row's button updates the state map; pressing again
  toggles back.
- `[Apply]` builds the resolution slice correctly: only off-default
  entries; create/update/delete rows are absent (default `ResolveAuto`).
- An `[Apply]` on a project with no drift/unknown rows produces an empty
  resolutions slice (auto behavior only).

## Acceptance Criteria

- [ ] Given a preview with rows of every `ChangeKind` plus one directory row,
      rendering the treetable yields four columns (Name, Status, Current
      Action, Actions) in that order; the directory row's Status and Current
      Action cells are blank — `TestPlanProjectRendersColumns`.
- [ ] For each row state in the button table, the rendered cell equals the
      expected `label`+mnemonic pair (drift→`[Overwrite o]`/`[Keep k]`,
      unknown→`[Delete d]`/`[Keep k]`, non-drift/unknown rows→empty) —
      `TestPlanProjectActionButtonMatrix`.
- [ ] Pressing the toggle mnemonic on a drift/unknown row flips its
      `Current Action` in the state map; pressing again reverts it —
      `TestPlanProjectToggleRoundTrip`.
- [ ] `[Apply]` on a preview with two off-default rows emits a resolution
      slice containing exactly those two paths; `[Apply]` on an all-default
      preview emits an empty slice —
      `TestPlanProjectApplyBuildsFileResolutions`.
- [ ] Walking every cursor position across every row kind, no two labelled
      buttons share a mnemonic from `{o,k,d,a,b}` —
      `TestPlanProjectMnemonicUniqueness`.

## Out of scope

- Apply behavior itself — covered in task 0017 (`sync.Apply` semantics).

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/tui/... -run 'TestPlanProjectRendersColumns|TestPlanProjectActionButtonMatrix|TestPlanProjectToggleRoundTrip|TestPlanProjectApplyBuildsFileResolutions|TestPlanProjectMnemonicUniqueness'`
  green.
- Smoke: `./bin/af` → Select Project Assets → Plan → toggle a drift row with
  `o`, an unknown row with `d`, press `[Apply]` → screen pops back and a
  `Project synced` toast appears.

## Plan

[plan.md](./plan.md)
