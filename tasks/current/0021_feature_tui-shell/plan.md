# Task 0021 — TUI shell rewrite (alt-screen + screen router)

Cross-links:
- [description.md](./description.md)
- Predecessor: [tasks/done/0015_task_refactor_ui/description.md](../../done/0015_task_refactor_ui/description.md) §Screens, §Notifications
- Dependencies (in-review): `tasks/current/0019_feature_actions-and-notifications/`, `tasks/current/0020_task_component-extensions/`
- Guidelines: `docs/guidelines/charm.md`, `docs/guidelines/tui.md`, `docs/guidelines/clean_architecture.md`, `docs/guidelines/clean_code.md`, `docs/guidelines/domain_model.md`, `docs/guidelines/solid.md`, `docs/guidelines/testing.md`, `docs/guidelines/go.md`
- Architecture: `docs/architecture/05-building-block-view.md`, `docs/architecture/06-runtime-view.md`
- ADRs touched: `docs/adr/0005-tui-as-the-only-user-interface.md`, `docs/adr/0006-remove-cli-subcommands-in-favor-of-pure-tui.md`, `docs/adr/0007-rendering-belongs-to-tui.md`. New ADR for the screen-router stack pattern (see Step 8).

## Context

The current `internal/tui/` is a synchronous `huh.NewForm` menu loop (`tui.go` → `mainMenu`/`profileMenu`/`assetMenu`/`projectMenu` → `forms.go::Run*` flows). Task 0015 defined a Bubble Tea alt-screen, screens-with-status-bar, FIFO toast UX that this loop cannot host. Tasks 0019 and 0020 landed the building blocks the new shell needs: `*actions.Actions` (typed wrapper around `app.Service`), `notifications.Toast`/`Log`/`NotificationMsg`/`From`, and `mnemonic.Set` (uniqueness-enforced button registry). Task 0021 is the shell itself — the root Bubble Tea program in alt-screen mode, a screen router (stack), global-key interception, status-bar composition, and a toast mount point above the status bar. The actual content screens (Welcome, Profiles, Edit Profile, …) are out of scope and arrive in tasks 0024–0029; we ship a placeholder `WelcomeStub` so the shell is observable and testable now.

## Design

### Package layout

New package `internal/tui/shell/`. Leaf-style: it imports `internal/actions`, `internal/tui/notifications`, `internal/tui/components/mnemonic`, `internal/tui/styles`, and Charm v2. No domain imports (`profile`, `project`, …) and no `internal/app`. Screens that show entities are added in 0024+ and live next to the shell, not inside it.

```
internal/tui/shell/
    screen.go       // Screen interface, PushScreenMsg, PopScreenMsg
    shell.go        // Model (root tea.Model), New(), Update, View
    keys.go         // global key bindings
    statusbar.go    // status-bar composition
    welcome_stub.go // placeholder Welcome screen
    shell_test.go
    welcome_stub_test.go
```

### The Screen interface

The task description prescribes an interface (`Screen { Init, Update, View, Title }`). This is intentional: a router *stack* needs a heterogeneous value type, which charm.md's "concrete typed sub-models" pattern does not cover (that pattern is for a single root view-enum, not a navigation stack). The two patterns coexist — the stack lives in the shell; each individual screen body that hosts multiple internal views is free to use the charm.md state-machine pattern inside its own `Update`.

```go
// internal/tui/shell/screen.go
package shell

import (
    tea  "charm.land/bubbletea/v2"
    "charm.land/bubbles/v2/key"
)

type Screen interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    Body(width, height int) string
    Title() string
    // StatusKeys returns the dynamic mnemonic bindings the status bar
    // appends to the global set. Empty when no row is selected/focused.
    StatusKeys() []key.Binding
}

type PushScreenMsg struct{ Screen Screen }
type PopScreenMsg struct{}
```

Notes:
- `Body(w, h int) string` instead of `View() string` so the shell controls layout and feeds each screen the content-area dimensions (full width minus header/toast/status-bar lines).
- `Title()` returns a plain string; styling (title-bar block) is owned by the shell.
- `StatusKeys()` is the mechanism §3 of the task description specifies. A screen returns the focused row's `mnemonic.Set.Buttons()` mapped to their `key.Binding` (via `Button.Binding()`), or `nil` when no row is selected. Screen-level labelled buttons are deliberately not exposed here — they are visible on the screen body and must not be duplicated in the bar (status-bar rule).

### Root Model (`shell.go`)

```go
type Model struct {
    actions *actions.Actions
    toast   *notifications.Toast
    log     *notifications.Log

    stack  []Screen
    keys   globalKeyMap

    width, height int
}

func New(a *actions.Actions, l *notifications.Log) Model
```

`New` constructs the toast with `notifications.NewToast(0)` (default 5 s), wires the log, and pushes `welcomeStub{}` as the only screen on the stack. `cmd/af/main.go` builds `app.Service`, then `actions.New(svc)`, then `notifications.NewLog()`, then `shell.New(...)`, then runs `tea.NewProgram(model)`.

`Init` returns the initial screen's `Init()`.

### `Update` routing order

This order is load-bearing — same precedence rules charm.md prescribes plus the task's "global keys fire even when a child screen would consume them":

1. `tea.WindowSizeMsg` → store `width`/`height`, forward to the top screen so tables/treetables can resize.
2. `tea.KeyPressMsg` →
   - `ctrl+c` returns `tea.Quit` unconditionally;
   - `handleGlobalKey(msg)` matches `n` / `s` / `q` / `?` via `key.Matches`; on hit, returns the corresponding cmd and **does not** forward to the active screen.
3. `notifications.NotificationMsg` → `log.Add(notif)` + forward as `PushMsg` to the toast.
4. `PushScreenMsg` → append to stack, run new screen's `Init()`.
5. `PopScreenMsg` → drop top (root stays — pop on a single-screen stack is a no-op).
6. **Default** → run `toast.Update(msg)` (eats only its own messages thanks to `expireMsg` being internal — the seq-guarded tick still routes back through the Bubble Tea runtime; `Toast.Update` ignores anything else) and then route to the top screen. Batch both returned commands.

### Global keys (`keys.go`)

```go
type globalKeyMap struct {
    Notifications key.Binding   // n
    Settings      key.Binding   // s
    Quit          key.Binding   // q
    Help          key.Binding   // ?
    Up            key.Binding   // ↑/k  (display-only; screens consume)
    Down          key.Binding   // ↓/j  (display-only; screens consume)
}
```

`handleGlobalKey` matches with `key.Matches` (never `msg.String()`). Up/Down bindings exist only so the status-bar can show them; the global handler returns `(nil, false)` for them so the active screen receives the press.

Mapping per task §4:
- `n` → push `notificationsStub` (real modal arrives in 0023; the shell already owns `*notifications.Log`, so the future modal just reads from it).
- `s` → push `settingsStub` (real screen in 0024).
- `q` → `tea.Quit`.
- `?` → push `infoStub` (real modal in 0023).

Stub screens are private to `shell` (lowercase types) and pop themselves on `esc`.

### `View` (`shell.go`)

```text
┌──────────────────────────────────────┐
│  {Title bar}                         │
└──────────────────────────────────────┘
{ body (full content area, screen-rendered) }

{ toast line — empty string when toast.Empty() → 0-height; otherwise 1 line }
{ status bar }
```

`View` uses `lipgloss.JoinVertical`, measures with `lipgloss.Height`, computes the body height as `m.height - headerH - toastLineH - 1` (status bar is one line), calls `top.Body(m.width, bodyH)`. `tea.NewView(...).AltScreen = true`.

### Status bar composition (`statusbar.go`)

```go
func renderStatusBar(global globalKeyMap, dynamic []key.Binding) string {
    parts := []string{
        keyHint(global.Up),
        keyHint(global.Down),
        keyHint(global.Notifications),
        keyHint(global.Settings),
        keyHint(global.Help),
        keyHint(global.Quit),
    }
    for _, b := range dynamic {
        parts = append(parts, keyHint(b))
    }
    return strings.Join(parts, "  ")
}

func keyHint(b key.Binding) string {
    // bubbles/key bindings already carry display text from
    // key.WithHelp(label, description); render "label description"
    // in muted style.
    h := b.Help()
    if h.Key == "" { return "" }
    return styles.MutedStyle.Render(h.Key + " " + h.Desc)
}
```

The "screen-level buttons are NOT repeated" rule is upheld by **what the screen exposes via `StatusKeys()`**: only the focused row's mnemonic-set bindings, never the screen's labelled-button verbs. The shell trusts the screen.

### Welcome stub (`welcome_stub.go`)

Minimal: title "Welcome", body `"agentfiles — press q to quit"`, `StatusKeys()` returns nil, `Update` is a no-op. Just enough to verify alt-screen + status bar + global keys end-to-end. Replaced by the real Welcome in task 0024.

### Stub screens for global-key destinations

Internal to `shell`. Each implements `Screen`, shows a placeholder body ("(notifications modal — task 0023)" etc.), pops on `esc`/`q`. Deleted in their respective tasks (0023/0024).

## Deletion list

After the shell compiles and tests pass, delete the legacy menu loop:

- `internal/tui/tui.go` — `Run`, `mainMenu`, `profileMenu`, `assetMenu`, `projectMenu`, `enterSubmenu`, `pause`, `runForm`, `errBack`, `errAlreadyReported`, `reportAction`.
- `internal/tui/forms.go` — every `RunProfile*`/`RunAsset*`/`RunProject*` plus `selectProfile`, `selectProfileAndProject`, `profileOptions`, `nonEmpty`, `supportedAssetTypes`, `supportedAgents`.
- `internal/tui/render_preview.go` and `internal/tui/render_preview_test.go` — only callers were `RunProjectPlan` / `RunProjectApply`. The Preview rendering will reappear inside the project-detail screens in task 0027+; not needed in the shell.
- `internal/tui/render_errors.go` and `internal/tui/render_errors_test.go` — only callers were `reportAction` and `RunProjectAdd`. The shell uses `notifications.From` (which carries severity + text into the toast already styled by `notifications/render.go::renderNotification`). If a later screen needs the multi-line error block, it can be reintroduced — for now, delete.
- `internal/tui/styles.go` — local re-aliases of `internal/tui/styles` package symbols used by the deleted render files. Delete.

Keep:
- `internal/tui/styles/` (shared palette).
- `internal/tui/notifications/` (untouched).
- `internal/tui/components/` (modal, mnemonic, treetable, focus, help, editor — all stay).

## `cmd/af/main.go` wiring

```go
func main() {
    registryPath := flag.String("registry", registry.DefaultPath(), "path to profile registry")
    flag.Parse()

    svc := app.New(*registryPath)
    a := actions.New(svc)
    log := notifications.NewLog()
    model := shell.New(a, log)

    if _, err := tea.NewProgram(model).Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

`tea.NewProgram` with no options — alt-screen is declared on `tea.View.AltScreen` per charm.md (Charm v2).

## Tests (`shell_test.go`)

Per the task's "Tests" section:

1. **Stack push/pop** — start with `[welcomeStub]`. Send `PushScreenMsg{settingsStub{}}` → top is `settingsStub`, depth 2. Send `PopScreenMsg{}` → depth 1, top is `welcomeStub`. Send `PopScreenMsg{}` again → depth stays 1 (root cannot be popped).
2. **Global key precedence** — install a `gluttonScreen` test screen whose `Update` returns a sentinel cmd for *every* key. Send a `tea.KeyPressMsg{Code: 'n'}` to the shell. Assert the returned cmd is the global "open notifications modal" cmd, not the sentinel. Repeat for `s`, `q`, `?`.
3. **Status-bar composition** — render the shell with an active screen whose `StatusKeys()` returns `[]{edit, delete}` bindings. Assert the status string contains `n notifications`, `s settings`, `q quit`, `? help`, `↑ up`, `↓ down`, `e edit`, `d delete`; assert it does **not** contain `c create`, `r register`, `b back` (the screen's labelled-button verbs — only valid if the screen does not surface them via `StatusKeys`, which is the rule).
4. **NotificationMsg → log + toast** — send `notifications.NotificationMsg{...}`. Assert log has one entry and `toast.Empty() == false`.
5. **WindowSizeMsg propagation** — send `tea.WindowSizeMsg{Width: 120, Height: 40}`. Assert shell stores 120/40 and the top screen received the same message (use a `sizingSpy` screen that records it).

`welcome_stub_test.go`: smoke test that `Body` returns the placeholder string and `Title` returns "Welcome".

## Step-by-step execution plan

1. **Branch + dependencies**
   - `git checkout -b feature/tui-shell` (from clean master).
   - Verify deps build: `go build ./internal/actions ./internal/tui/notifications ./internal/tui/components/mnemonic`.

2. **`internal/tui/shell/screen.go`** — `Screen` interface, `PushScreenMsg`, `PopScreenMsg`.

3. **`internal/tui/shell/keys.go`** — `globalKeyMap` struct + `defaultGlobalKeyMap()` factory binding `n`, `s`, `q`, `?`, plus display-only `↑/k` and `↓/j` for the status bar. `(Model) handleGlobalKey(tea.KeyPressMsg) (tea.Cmd, bool)`.

4. **`internal/tui/shell/welcome_stub.go`** + sibling private stubs `notificationsStub`, `settingsStub`, `infoStub`. `welcome_stub_test.go` covers `Body` + `Title`.

5. **`internal/tui/shell/statusbar.go`** — `renderStatusBar(globalKeyMap, []key.Binding) string`.

6. **`internal/tui/shell/shell.go`** — `Model`, `New(*actions.Actions, *notifications.Log)`, `Init`, `Update`, `View`. Alt-screen enabled in `View`.

7. **`internal/tui/shell/shell_test.go`** — the five tests listed above.

8. **`cmd/af/main.go` rewrite** — wire `app → actions → log → shell → tea.NewProgram`.

9. **Delete legacy** — remove `internal/tui/tui.go`, `forms.go`, `render_preview.go` (+ test), `render_errors.go` (+ test), `styles.go`. `make build` to confirm no other references.

10. **Documentation**
    - **`docs/architecture/05-building-block-view.md`**: update `tui` section (remove "huh menu / runForm" paragraph; describe shell + screen router + status bar + toast mount); update the dependency diagram (`tui → app` becomes `tui → actions → app`; add the `tui → tui/notifications` and `tui → tui/components/mnemonic` arrows); add a Level-2 entry for `internal/tui/shell`.
    - **`docs/architecture/06-runtime-view.md`**: the "top-level menu" prose is now stale; revise so each scenario starts at the relevant screen rather than the huh menu. Leave the Plan/Apply sequence diagram intact (it describes app/render/sync, not the TUI shape).
    - **New ADR** `docs/adr/0011-tui-screen-router.md`: documents the screen-stack approach for the shell, explains coexistence with charm.md's "concrete sub-models per entity" pattern, and records the global-key/dynamic-mnemonic split for the status bar. Update `docs/adr/README.md` to list it.
    - **No new guidelines file** — `charm.md` and `tui.md` already cover the building blocks; the screen-router pattern is task-specific and belongs in an ADR, not a guideline.

11. **Quality gate** — `make fmt && make build && make test && make lint`. Manual smoke: `./bin/af` — alt-screen opens; Welcome title shows; status bar reads `↑ up  ↓ down  n notifications  s settings  ? help  q quit`; `n`/`s`/`?` push stubs (each `esc` pops); `q` quits cleanly; `ctrl+c` quits cleanly.

12. **Set `description.md` status → `in-review`**, write changelog `docs/changelog/2026-06-11_0021-tui-shell.md` per template.

## Verification

```
make build && make test && make lint
./bin/af
```

In `./bin/af`:
- alt-screen activates (cursor at top, terminal scrollback preserved on exit);
- title bar shows "Welcome";
- status bar shows global hints in order: `↑ up`, `↓ down`, `n notifications`, `s settings`, `? help`, `q quit`;
- `n` pushes Notifications stub; `esc` pops;
- `s` pushes Settings stub; `esc` pops;
- `?` pushes Info stub; `esc` pops;
- `q` and `ctrl+c` quit cleanly.

## Critical files referenced

Existing (read / wire to):
- `internal/actions/actions.go` — `actions.New(*app.Service) *Actions`
- `internal/tui/notifications/log.go` — `NewLog()`, `Add`, `Entries`
- `internal/tui/notifications/toast.go` — `NewToast`, `PushMsg`, `Empty`
- `internal/tui/notifications/bridge.go` — `NotificationMsg`, `From`
- `internal/tui/components/mnemonic/set.go` — `Set`, `Button`
- `internal/tui/styles/styles.go` — `HeaderStyle`, `MutedStyle`, `SeverityStyle`

To delete (after new shell builds):
- `internal/tui/tui.go`
- `internal/tui/forms.go`
- `internal/tui/render_preview.go` (+ test)
- `internal/tui/render_errors.go` (+ test)
- `internal/tui/styles.go`

To create:
- `internal/tui/shell/{screen,shell,keys,statusbar,welcome_stub,shell_test,welcome_stub_test}.go`
- `docs/adr/0011-tui-screen-router.md`
- `docs/changelog/2026-06-11_0021-tui-shell.md`

To update:
- `cmd/af/main.go`
- `docs/architecture/05-building-block-view.md`
- `docs/architecture/06-runtime-view.md`
- `docs/adr/README.md`
- `tasks/current/0021_feature_tui-shell/description.md` (frontmatter → `in-progress` at impl start, → `in-review` at end; append `## Plan` link)
