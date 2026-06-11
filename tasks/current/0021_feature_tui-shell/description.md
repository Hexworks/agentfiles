---
id: 0021
type: feature
status: in-review
topics: go, tui, charm
depends_on: 0019, 0020
---

# TUI shell rewrite (alt-screen + screen router)

This task replaces the existing `huh`-based menu loop with the alt-screen
Bubble Tea program described in `0015_task_refactor_ui/description.md`,
sections **Screens** (template + status bar) and **Notifications** (area
placement).

## Background

`internal/tui/tui.go` is a synchronous loop over `huh.NewForm` submenus
(`mainMenu` → `profileMenu` / `assetMenu` / `projectMenu`). The legacy
`forms.go` file hosts the flow functions (`RunProfileCreate`,
`RunProjectAdd`, `RunAssetInit`, etc.) plus the `render_*.go` helpers
(`RenderError`, `RenderPreview`). The new TUI is a single
long-running Bubble Tea program in alt-screen mode with its own screen
router; the legacy entry points no longer fit.

## Scope

### 1. Alt-screen Bubble Tea root model

- New package or file (suggestion: `internal/tui/shell` or rewrite of
  `tui.go`) hosts the root `tea.Model`.
- Program runs in alt-screen mode.
- `tea.WindowSizeMsg` is captured and propagated; screens receive a sizing
  hook so tables/treetables can size themselves to fit.

### 2. Screen router (stack)

- Screens implement a small interface (e.g. `Screen { Init, Update, View,
  Title }`) and may push/pop other screens.
- Push/pop is message-driven: a screen returns a `PushScreenMsg{Screen}` or
  `PopScreenMsg` from its `Update`, the root handles the transition.
- Initial route: a placeholder `WelcomeStub` screen (will be replaced by
  task 0024) so the shell is testable before the real Welcome lands.

### 3. Status bar

Per parent task:

- **Always shows global keys**: `n notifications`, `s settings`, `q quit`,
  `?` help, plus `↑/k`, `↓/j`.
- **Dynamic**: when a row is selected and its table is focused, append the
  mnemonic keys exposed by that row's `mnemonic.Buttons` (e.g.
  `e edit`, `d delete`).
- **Screen-level buttons are NOT repeated**. Buttons rendered in the screen
  body (e.g. `[Create New Profile]`) are visible as labelled buttons; the
  status bar must not duplicate them.

Implementation: the active screen exposes its current `mnemonic.Set` (from
task 0020) plus a static-key slice via a small Screen method
(`StatusKeys() []key.Binding`); the shell joins those with the global set.

### 4. Global key bindings

Handled at the shell level, intercepted before the active screen sees them:

- `n` — open Notifications Modal (placeholder until task 0023 wires it).
- `s` — push Settings Screen (placeholder until task 0024).
- `q` — quit.
- `?` — open Info Modal (placeholder until task 0023).

### 5. Notification area + toast queue mount point

- The toast widget (`notifications.Toast` from task 0019) is mounted just
  above the status bar in every screen.
- The notification area is a single line. Empty → invisible (the line is
  not rendered).

### 6. Remove legacy

Delete:

- `internal/tui/tui.go` menu loop (`Run`, `mainMenu`, `profileMenu`,
  `assetMenu`, `projectMenu`, `enterSubmenu`, `pause`).
- `internal/tui/forms.go` flow functions and any helper functions only
  reachable from them.
- `internal/tui/render_*.go` only if their callers are gone. `RenderError`
  may still be used by the new error toast helper; keep it if so.
- The corresponding `_test.go` files.

`cmd/af/main.go` switches over: it builds the Service, builds
`*actions.Actions` (task 0019), and starts the new shell.

## Tests

- Root model `Update` handles `PushScreenMsg`/`PopScreenMsg` correctly
  (stack push/pop, no leaks).
- Global key bindings fire even when a child screen would consume them
  (precedence test).
- Status bar composition: global + dynamic mnemonics joined as expected,
  screen-level buttons absent.

## Out of scope

- The actual Welcome/Settings/Profiles/Edit Profile/etc. screens
  (tasks 0024–0029). Initial placeholder is fine.
- The Notifications and Info modal *bodies* (task 0023). The global key
  handlers may dispatch to no-ops until then.

## Verification

```
make build && make test && make lint
./bin/af   # opens alt-screen with placeholder Welcome; n/s/q/? work
```

## Plan

[plan.md](./plan.md)
