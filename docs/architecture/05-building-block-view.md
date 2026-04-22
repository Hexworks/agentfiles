# 5. Building Block View

The implementation is split into small packages around the domain model and the
main workflow.

## Top-Level Building Blocks

### `registry`

Loads and saves `~/.agentprofiles.json`, resolves profiles, and tracks metadata
such as last-opened timestamps.

### `profile`

Initializes and loads profile folders. A profile contains `profile.json`,
`assets/`, and `projects/`.

### `asset`

Defines asset types, validates asset manifests, and scaffolds new assets. Asset
types currently include `skill`, `agents_doc`, `settings`, `mcp`, `rule`, and
`hook`.

### `project`

Defines per-project manifests containing the target path, enabled agents, and
selected asset ids.

### `render`

Builds a project plan by resolving selected assets and projecting them into
agent-specific output paths.

### `sync`

Calculates preview changes, detects drift, detects recognized delete
candidates, writes files, and stores managed state in `.agentfiles/state.json`.

### `app`

Coordinates the higher-level operations used by the TUI, including profile
creation, project ownership checks, planning, and apply.

### `tui`

Implements every interactive flow on top of `huh`: the top-level menu, the
per-category submenus, and one form per command. Free-form fields use text
inputs while closed sets (asset types, supported agents, registered profiles,
profile-owned projects, profile-owned assets) use Select / MultiSelect
populated from the domain layer. Every command lives here; no other package
collects user input. A local `runForm` helper installs a keymap that treats
Esc the same as Ctrl+C so every prompt aborts consistently when the user
wants to back out one level.

### `cmd/af`

The binary entry point. It parses the single `--registry` flag with the
standard-library `flag` package and calls `tui.Run`. There is no Cobra
command tree and no intermediate routing package — `af` always opens the
TUI.

