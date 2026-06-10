---
id: 0028
type: feature
status: pending
depends_on: 0021
---

# Select Project Assets screen

Implements **Select Project Assets Screen** from
`0015_task_refactor_ui/description.md`. Two equal-size tables stacked
vertically: Selected Assets (top), Available Assets (bottom). Each
row-level select/unselect immediately calls `UpdateProject`.

## Parameters

- `id`: the project's id, passed when the screen is pushed.

## Data load

On `Init`, run the `LoadProject(id)` action (task 0019). Also load the
parent profile (already in screen state if pushed from Edit Profile;
otherwise fetch via `LoadProfile`) to get the full asset list.

## Layout

```
┌────────────────────────────────────────────────────────────────┐
│Selecting assets for project {{project.name}} ({{profile.name}})│
└────────────────────────────────────────────────────────────────┘
┌[1]─Selected─Assets──────────────────────────────────────────────┐
│ Id   Name           Type        Exclusive Group   Actions       │
│ #1   AGENTS.md      agents_doc  agents_doc        [Unselect]    │
│ #2   review-code    skill                                       │
└─────────────────────────────────────────────────────────────────┘
┌[2]─Available─Assets─────────────────────────────────────────────┐
│ Id   Name           Type        Exclusive Group   Actions       │
│ #1   create-task    skill                         [Select]      │
│ #2   do-task        skill                                       │
│ #3   AGENTS2.md     agents_doc  agents_doc                      │
└─────────────────────────────────────────────────────────────────┘
                                                    [Plan] [Back]

 {{ notification area }}
 ↑/k up • ↓/j down • n notifications • s settings • q quit • ? help
```

## Focus model

Two `bubbles/table` widgets in a `focus.Handler`:

- `[1]` (`ctrl+1`) focuses Selected Assets.
- `[2]` (`ctrl+2`) focuses Available Assets.
- `tab` / `shift+tab` cycle.

## Selected Assets table

Columns: `Id`, `Name`, `Type`, `Exclusive Group`, `Actions`.

Row action (selected + focused):

- `u` `[Unselect]` → remove the asset id from the project's
  `SelectedAssetIDs`; call `UpdateProject` action.

## Available Assets table

Columns: identical.

Row action (selected + focused):

- `l` `[Select]` → add the asset id to the project's `SelectedAssetIDs`;
  call `UpdateProject` action.

## Screen-level buttons (below the bottom table, right-aligned)

- `[Plan]` (mnemonic `p`) → push **Plan Project Screen** (task 0029) with
  the project id.
- `[Back]` (mnemonic `b`) → pop to Edit Profile.

## Sizing

Heading + button row + notification area + status bar fixed; the two tables
split the remaining space equally.

## Note (per parent task)

> The Plan screen will use the current state that exists. Actions performed
> on this screen are automatically saved (select/unselect).

That is satisfied because every row action calls `UpdateProject`
immediately — no batched commit.

## Status bar

Global keys + selected-row mnemonics of the focused table. Screen-level
`p`/`b` excluded.

## Tests

Per the safety rule, walk every focus + selection state and assert mnemonic
uniqueness. Candidate alphabet: `{1, 2, u, l, p, b}`.

Plus:

- Select an asset → moves it from Available to Selected; `UpdateProject`
  is called with the new id list.
- Unselect an asset → reverse.
- Reopening the screen after a select shows the persisted state.

## Out of scope

- Plan Project Screen body (task 0029).

## Verification

```
make build && make test && make lint
./bin/af   # Edit Profile → Project row `a` → Select / Unselect / Plan / Back
```
