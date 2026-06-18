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

## Directory actions (register as asset)

When the cursor sits on a **directory row whose every descendant file is
`? unknown`**, a **Register** (`r`) action appears. It opens the Create Asset
modal pre-filled with the folder's name and a caption showing the folder's file
count and size. The type picker is limited to the convention types that own a
folder (`skill`, `agents_doc`, `settings`); pick a type and confirm to:

1. create a new asset under the active profile,
2. **copy** the folder's files into the profile's `assets/`, and
3. select that asset for the current project.

The plan reloads in place (the screen does not pop), so the folder's files are
re-classified from `? unknown` to managed `+ add` / `~ update` rows. The
original project files are left untouched.

Directories with any managed (`add` / `update` / `drift`) leaf do not offer
**Register** — they are already partly owned by the profile.

## Directory actions (ignore folder)

The same all-unknown directory rows also offer **Ignore** (`i`) alongside
**Register**. Ignoring is an in-memory toggle: the folder row collapses (its
subtree disappears and the trailing `/` drops from the label), **Register**
hides, and the button flips to **Show** (`s`) to restore it. No modal appears.

On **Apply**, ignored folders persist into the `ignored_paths` list in
`<repo>/.agentfiles/state.json` (unioned with any already-persisted paths, never
dropped). Every later **Plan** suppresses any `? unknown` whose path sits under
an ignored folder, so the folder no longer appears at all.

## Screen actions

- **Apply** (`a`) — write the plan. On success the screen pops back to where
  you came from with a toast.
- **Back** (`b` / `esc`) — return without applying.

## Notes

`add` / `update` / `delete` are always executed as shown — only `drift` and
`unknown` accept per-row resolutions.
