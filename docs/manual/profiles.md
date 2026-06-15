# Profiles

Lists every profile registered in `~/.agentprofiles.json`. A profile owns the
canonical assets (skills, hooks, MCP, rules, agents docs) and the projects they
project into.

## Operations

- **Create New Profile** (`c`) — scaffold a fresh profile folder.
- **Register Profile** (`r`) — point the registry at an existing folder on disk.
- **Edit** (`e`) — open the highlighted profile in **Edit Profile**.
- **Delete** (`d`) — two-step confirm: remove the registry entry, then optionally
  delete the folder on disk.
- **Back** (`b` / `esc`) — return to Welcome.

## Navigation

- `↑`/`k`, `↓`/`j` — move the cursor between profile rows.
- `e` / `d` act on the cursor row.
- `e` pushes **Edit Profile** for the highlighted profile.

## Notes

A target repo path may belong to at most one profile — register the same path
twice and the second attempt fails with a typed error.
