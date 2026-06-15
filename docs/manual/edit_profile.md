# Edit Profile

Two-panel screen: the **Assets** panel on top, **Projects** below. Edit either
the asset catalog or the per-project asset selection and metadata.

## Focus

- `tab` / `shift+tab` — switch between Assets and Projects.
- `↑`/`k`, `↓`/`j` — move the cursor in the focused table.

## Assets panel — row actions

- **Edit** (`e`) — push **Edit Asset** for the highlighted asset.
- **Delete** (`d`) — confirm, then remove the asset (and unselect it from every
  project that referenced it).

## Projects panel — row actions

- **Edit** (`e`) — open a form to rename / re-path / re-target agents.
- **Select Assets** (`a`) — push **Select Project Assets** to pick which assets
  this project projects.
- **Plan** (`p`) — push **Plan Project** to preview and apply the projection.
- **Delete** (`d`) — confirm; the manifest is removed but rendered files stay
  on disk as orphans.

## Screen actions

- **Create Asset** (`c`) — modal form to scaffold a new asset.
- **Register Project** (`r`) — modal form to add a new project manifest.
- **Back** (`b` / `esc`) — return to **Profiles**.
