# Plan Project

Preview every file change `af` would make in the target repo for this project
and apply it. Rows show `Status` and `Current Action`.

## Status legend

- `+ add` — new managed file.
- `~ update` — managed content changed.
- `- delete` — managed file removed from desired set.
- `* drift` — local file's hash diverged from the last recorded state.
- `? unknown` — file lives under a managed surface but is not in the desired
  set and was not tracked before.

## Row actions (drift / unknown only)

The cursor row shows a toggle button reflecting the **other** option:

- Drift rows: **Overwrite** (`o`) ↔ **Keep** (`k`).
- Unknown rows: **Delete** (`d`) ↔ **Keep** (`k`).

Default for both is **Keep**.

## Screen actions

- **Apply** (`a`) — write the plan. On success the screen pops back to where
  you came from with a toast.
- **Back** (`b` / `esc`) — return without applying.

## Notes

`add` / `update` / `delete` are always executed as shown — only `drift` and
`unknown` accept per-row resolutions.
