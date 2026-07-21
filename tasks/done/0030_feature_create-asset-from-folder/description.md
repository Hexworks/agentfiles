---
id: 0030
type: feature
status: done
topics: tui, profile, asset
depends_on: 0029
---

# Create Asset From Folder

On the **Plan Project** screen (task 0029) the treetable lists changes. An
agent directory can contain folders (e.g. `.claude/skills/something`) that
classify as `? unknown` (`ChangeUnknown`) — unmanaged files the profile does
not own.

Add an action **on the folder row** itself: **Register as Asset**, mnemonic
`r`. Pressing it opens the existing **Create Asset** modal pre-scoped to that
folder (the files under `something/`) so the user can configure the asset and
add it to the profile.

## Behaviour

- The `r` action is offered only on directory rows whose contents are
  unknown/unmanaged (not on managed rows, not on individual files).
- Selecting `r` opens the Create Asset modal. The asset's source is the
  selected folder; its files become the asset's content.
- On submit:
  - A new `Asset` is created under the active profile (`asset.json` +
    scaffolding), following the asset-authoring rules
    (`docs/guidelines/asset_authoring.md`).
  - The files in the selected folder are **copied** into the profile's
    `assets/` so they become profile-owned and can sync to other projects.
  - The new asset is selected for the current project so a subsequent plan
    classifies those files as managed (`create`/`update`) instead of
    `? unknown`.

## Out of scope

- Changing how `ChangeUnknown` is classified (task 0017).
- Moving/deleting the original files from the project (they become managed on
  the next sync, not relocated here).

## Verification

```
make build && make test && make lint
./bin/af   # Plan Project → cursor on an unknown folder → r → fill modal → submit
```

## Plan

[plan.md](./plan.md)
