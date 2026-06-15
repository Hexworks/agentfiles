# Select Project Assets

Pick which of the profile's assets this project projects. Two tables: **Selected
Assets** on top, **Available Assets** below. Every action persists immediately.

## Focus

- `tab` / `shift+tab` — switch between Selected and Available.
- `↑`/`k`, `↓`/`j` — move the cursor in the focused table.

## Row actions

- **Unselect** (`u`) — remove the highlighted Selected asset from the project.
- **Select** (`l`) — add the highlighted Available asset. Note: the mnemonic is
  `l`, not `s` (`s` is the global Settings shortcut).

## Screen actions

- **Plan** (`p`) — push **Plan Project** to preview the projection against the
  current selection.
- **Back** (`b` / `esc`) — return to **Edit Profile**.

## Notes

Compatibility and `exclusive_group` rules are enforced server-side: an asset
incompatible with the project's enabled agents — or one that collides with an
already-selected asset's exclusive group — will be rejected with a typed error
shown as a notification.
