# TUI Screen Router

## Status

accepted

## Context

Before task 0021, `internal/tui/tui.go` ran a synchronous
`huh.NewForm` menu loop: `mainMenu → profileMenu / assetMenu /
projectMenu → forms.go::Run*`. Each Run\* flow was a self-contained
sequential dialog; the loop only orchestrated which dialog to start.

Task 0015 specified a new UI: an alt-screen Bubble Tea program with
per-entity screens, a persistent title bar, a single-line toast area
for notifications, and a dynamic status bar. Tasks 0019 (actions +
notifications) and 0020 (component extensions) shipped the building
blocks: `*actions.Actions` (the typed wrapper screens use instead of
`*app.Service` directly), `notifications.Toast` / `Log` /
`NotificationMsg` / `From`, and `mnemonic.Set`.

The synchronous menu loop cannot host these. The new shell must:

1. run in alt-screen mode (charm.md mandates this for multi-view apps);
2. navigate between an unbounded number of screens, where one screen
   can push another (e.g. Profiles → Edit Profile → Edit Asset), and
   the previous screens stay alive underneath;
3. intercept four global keys (`n` notifications, `s` settings, `?`
   info, `q` quit) before the active screen consumes them;
4. mount a toast widget above the status bar that any action's
   bridge command can feed into, regardless of which screen is
   active;
5. compose a status bar from a fixed global set plus dynamic
   per-screen mnemonic bindings.

The challenge: `docs/guidelines/charm.md` describes the canonical
multi-view pattern as a state machine — a view enum on the root model
and concrete typed sub-model fields ("`profile profileView`,
`project projectView`, …"), routed by inspecting `m.view`. That
shape is right when the *set* of views is closed and known up-front.
It is wrong for an open stack of arbitrary depth where a screen does
not statically know what screen pushed it.

## Decision

The shell uses an interface-typed **screen-router stack** as its
navigation primitive. The stack and the per-entity state-machine
pattern coexist; they answer different questions.

`internal/tui/shell/screen.go` defines:

```go
type Screen interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    Body(width, height int) string
    Title() string
    StatusKeys() []key.Binding
}

type PushScreenMsg struct{ Screen Screen }
type PopScreenMsg struct{}
```

`shell.Model` owns `stack []Screen`; the top of the stack is the
active screen. Screens never mutate the stack directly — they emit
`PushScreenMsg` or `PopScreenMsg` from their `Update`, and the root
applies the transition. The root cannot be popped (a `PopScreenMsg`
on a single-element stack is a no-op).

`Update` routing order is fixed:

1. `tea.WindowSizeMsg` — store on the shell, forward to the active
   screen so tables/treetables can resize.
2. `tea.KeyPressMsg` — `ctrl+c` returns `tea.Quit` unconditionally;
   `handleGlobalKey` matches the global set with `key.Matches`. On
   hit, return the corresponding cmd and **do not** forward to the
   screen.
3. `notifications.NotificationMsg` — write to the log, forward as
   `PushMsg` to the toast.
4. `PushScreenMsg` / `PopScreenMsg` — mutate the stack, run the new
   top screen's `Init` on push.
5. Default — feed the toast (it ignores anything that isn't its own
   `PushMsg` / `expireMsg`) and forward to the active screen. Batch
   both returned commands.

The status bar (`statusbar.go`) joins the global hints with
`top.StatusKeys()`. The "screen-level labelled buttons must not be
duplicated in the bar" rule from task 0015 is upheld at the screen
boundary: a screen's `StatusKeys()` returns only the focused row's
mnemonic bindings, never the screen's labelled-button verbs. The
shell trusts the contract.

Screens that internally host more than one view (the future Profiles
screen, say, with list + create dialog + delete confirm) follow the
charm.md sub-model pattern *inside* their own `Update`. The two
patterns layer cleanly: the stack handles navigation between
*entities*; the state machine handles transitions *within* a screen.

## Consequences

The shell is small and testable. Five focused tests cover the
contract: stack push/pop (including root protection), global-key
precedence over the active screen (with a recording screen that
asserts non-receipt), `NotificationMsg → log + toast` plumbing,
`WindowSizeMsg` propagation, and status-bar composition.

Three placeholder stubs (`notificationsStub`, `settingsStub`,
`infoStub`) ship inside the shell package so the global keys can
push something the user sees before tasks 0023 and 0024 land. They
pop on `esc` and will be replaced wholesale by the real modal /
screen implementations in those tasks. They live next to the shell
because they are the shell's own glue, not standalone entity views.

The `Screen` interface adds dynamic dispatch to the navigation hot
path. Each `Update` cycle performs one virtual method call to route
into the active screen — negligible cost; the alternative (a closed
view enum) would prevent third-party screens from being added by
later tasks without modifying the shell, which is the precise
extension point this design exists to provide.

The shell deletes the legacy `tui.go` menu loop, `forms.go` flows,
and the `render_preview.go` / `render_errors.go` helpers. Preview
rendering will return inside the project-detail screens in
task 0027+; error rendering moves wholesale to the toast pipeline
(`notifications.From` carries `err.Severity()` + `err.Error()` into
the toast already styled by `notifications/render.go`).

## References

- `internal/tui/shell/screen.go`, `shell.go`, `keys.go`,
  `statusbar.go`, `welcome_stub.go`
- `docs/guidelines/charm.md` § "Multi-View State Machines",
  § "Composing Sub-Models"
- `tasks/done/0015_task_refactor_ui/description.md` § Screens,
  § Notifications
- ADR 0005, 0006 (TUI as only UI; CLI subcommands removed)
- ADR 0007 (Rendering belongs to TUI)
