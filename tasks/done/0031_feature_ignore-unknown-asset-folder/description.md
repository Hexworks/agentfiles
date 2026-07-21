---
id: 0031
type: feature
status: done
topics: tui, asset_authoring, charm
---

# Ignore unknown asset folder

On the **Plan Project** screen the "Changes" treetable lists folders that
classify as `? unknown` (`ChangeUnknown`) — unmanaged directories the profile
does not own. Task 0030 lets the user **Register** such a folder as an asset.
This task adds the opposite escape hatch: **Ignore** the folder so it stops
appearing as a change.

Ignoring is a per-project, in-memory resolution (like the existing drift and
unknown toggles). It is only persisted to disk when the plan is **applied**,
into a new `ignored_paths` list in the target repo's `.agentfiles/state.json`
(`sync.ManagedState`), alongside `managed_files`.

## Behaviour

### The Ignore toggle (folder rows)

- On a directory row whose contents are entirely unknown (`dirAllUnknown`), the
  folder currently offers **Register** (mnemonic `r`). Add an **Ignore** action,
  mnemonic `i`.
- Ignore is a **toggle** with no confirmation modal — consistent with the drift
  (`Keep`/`Overwrite`) and unknown (`Keep`/`Delete`) toggles. Nothing is
  destructive until Apply, which already previews.
- While a folder is selected for ignoring:
  - Its action button flips to its counterpart **Show** (un-ignore),
    mnemonic `w` (`s` is reserved for the global Settings action).
  - **Register** is hidden — we cannot register a folder we are ignoring.
  - The folder's children are **collapsed**: the row renders without its
    subtree and drops the trailing `/` on the label. The child file rows (and
    their `Open` / `Keep` / `Delete` buttons) disappear while ignored.

  ```
  │     └── skills/                                                          │
  │         ├── ask-matt/                                  [Register]        │
  │         │   └── SKILL.md                                                 │
  ```

  becomes

  ```
  │     └── skills/                                                          │
  │         ├── ask-matt                                   [Show]            │
  ```

- The collapse is achieved by rebuilding the tree (`buildPlanTree` →
  `SetRoot`) with knowledge of the in-memory ignored set, omitting the
  ignored folder's `Children`. No change to the `treetable` component.

### Persistence (on Apply)

- `sync.ManagedState` gains `IgnoredPaths []string` (`json:"ignored_paths"`).
  Paths are stored **repo-relative** (matching the existing `managed_files`
  form, e.g. `.claude/skills/ask-matt`).
- When Apply runs, every in-memory ignored folder is written into
  `ignored_paths`.
- On the **next** Plan, `sync.Plan` suppresses any `ChangeUnknown` whose path
  is under an ignored folder (path-prefix match). The folder therefore vanishes
  from the "Changes" table entirely — no row, no `Show` toggle.

## Out of scope

- Changing how `ChangeUnknown` is classified (task 0017).
- A surface for viewing / un-ignoring already-persisted `ignored_paths`. Once a
  folder is persisted and filtered it can only be restored by hand-editing
  `state.json`; a "manage ignored paths" screen is a **follow-up task**.
- Registering or otherwise mutating the ignored folder's files on disk.

## Verification

```
make build && make test && make lint
./bin/af   # Plan Project → cursor on an unknown folder → i → row collapses, shows [Show]
           # → Apply → reopen Plan → folder no longer listed
```

## Plan

[plan.md](./plan.md)
