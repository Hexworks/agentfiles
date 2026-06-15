# TUI shell rewrite review

The new `internal/tui/shell` package compiles, all 21 new tests pass, and the deletion of the legacy `huh` menu loop is clean. The shell honors most of the task's design constraints (alt-screen, screen-router stack, global-key precedence, toast above status bar). The findings below cluster around four themes:

1. **Unbounded growth + input sanitization** — the toast queue, the screen stack, and the status-bar binding text all skip the same safety wrap that the rest of the TUI package consistently applies.
2. **Routing precedence inconsistency** — `ctrl+c` is the one global key the shell handles via raw string compare, breaking the `key.Matches` discipline charm.md mandates and that every other global binding follows.
3. **`Screen` interface ergonomics** — every shipped screen returns `nil` from `StatusKeys`, and every stub repeats the same `esc → pop` boilerplate; both signal an interface and a type taxonomy that can be tightened.
4. **Test scoping + coverage gaps** — shell tests instantiate a real `app.Service` despite no `app` dependency, several documented behaviors (PushScreen `Init` cmd, default-route fan-out, View body-height math, multi-stack `WindowSizeMsg`) are untested, and one composite test mixes two behaviors.

A few smaller items (doc comments, file naming, stale arrows in the building-block diagram) are below the substantive concerns but worth catching now.

## Unbounded toast queue

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) ("Treat External Input As Untrusted" / resource exhaustion)

`Toast.Update`'s `PushMsg` branch unconditionally appends to `t.queue`. The shell forwards every `NotificationMsg` into it. An accumulator-shape action that dispatches one toast per leaf error, or any flap loop emitting notifications faster than 5 s, grows the queue without bound and forces the user to wait `n×5s` for the backlog to drain.

```go
// internal/tui/notifications/toast.go:54
case PushMsg:
    becameFront := len(t.queue) == 0
    t.queue = append(t.queue, m.Notification) // no cap; queue grows forever
```

- [ ] Apply a hard cap (e.g. `LogCap/10`) and drop oldest pending when exceeded.
- [ ] Replace FIFO with "current + 1 pending"; new pushes overwrite the pending slot.
- [x] Leave as-is and document the rationale (queue size is bounded by upstream action volume which is itself human-paced).

## Unbounded screen stack

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) (resource exhaustion / unbounded growth)

`PushScreenMsg` appends to `m.stack` with no cap and no dedup. The three global keys (`n`, `s`, `?`) each push a fresh stub on every press; auto-repeat holds the key down and grows the stack one screen per keystroke. `PopScreenMsg` pops one at a time, so the user must press `esc` once per accumulated push to drain.

```go
// internal/tui/shell/shell.go:77
case PushScreenMsg:
    m.stack = append(m.stack, msg.Screen) // unbounded; no dedup
    return m, msg.Screen.Init()
```

- [x] No-op the push when the new screen's concrete type equals the current top (the three global stubs are idempotent by nature).
- [ ] Cap depth (e.g. 16) and ignore further pushes.

## `ctrl+c` matched via `msg.String()` instead of `key.Matches`

> [!WARNING]
>
> - [docs/guidelines/charm.md](../../../docs/guidelines/charm.md) §"Key Bindings — Consistent and Visible" ("match keys with `key.Matches`, not by `msg.String()`")
> - This task's own [plan.md](./plan.md) §"Global keys" ("matches with `key.Matches` (never `msg.String()`)")

Every other shell-level binding (`n`, `s`, `?`, `q`, `↑`, `↓`) goes through `key.NewBinding(...)` and `key.Matches` via `globalKeyMap` / `handleGlobalKey`. `ctrl+c` alone splits the policy with a raw string compare and is invisible to `globalKeyMap`, so the status bar / help cannot surface it and it cannot be rebound.

```go
// internal/tui/shell/shell.go:63-66
case tea.KeyPressMsg:
    if msg.String() == "ctrl+c" {
        return m, tea.Quit
    }
    if cmd, handled := m.handleGlobalKey(msg); handled {
```

- [x] Add `ctrl+c` to `globalKeyMap.Quit` as a second key: `key.WithKeys("q", "ctrl+c")` and drop the inline compare.
- [ ] Add a separate `Interrupt key.Binding` to `globalKeyMap` (`key.WithKeys("ctrl+c")`) routed through `handleGlobalKey`.
- [ ] Keep the inline check but match on the structured fields: `msg.Mod == tea.ModCtrl && msg.Code == 'c'`.

## Status-bar binding text not run through `styles.Safe`

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) ("Treat External Input As Untrusted")
> - [docs/adr/0007-rendering-belongs-to-tui.md](../../../docs/adr/0007-rendering-belongs-to-tui.md) (the `safe()` helper precedent for manifest-sourced strings)

`keyHint` renders `h.Key + " " + h.Desc` straight through `lipgloss` without `styles.Safe`. Today all bindings come from in-source literals, so the strings are trusted. The boundary is fragile: as soon as a real screen sources a `StatusKeys` binding's `Help` text from a profile manifest (asset name, button label, etc.) — exactly the dynamic-mnemonic purpose — an untrusted string with bidi overrides or C1 controls reaches the terminal raw. The notification renderer already applies `Safe`; the status bar should too.

```go
// internal/tui/shell/statusbar.go:37
func keyHint(b key.Binding) string {
    h := b.Help()
    if h.Key == "" { return "" }
    return styles.MutedStyle.Render(h.Key + " " + h.Desc) // no Safe()
}
```

- [x] Wrap: `styles.MutedStyle.Render(styles.Safe(h.Key) + " " + styles.Safe(h.Desc))`.
- [ ] Document at the `Screen.StatusKeys` contract that callers must pre-sanitize binding text.

## `WindowSizeMsg` only reaches the top screen

> [!WARNING]
>
> - [docs/guidelines/charm.md](../../../docs/guidelines/charm.md) §"Window Size Propagation" ("The root model must catch it and push the new dimensions through to every component that has its own layout")

Screens buried in the stack miss every resize that fires while they sit beneath the top. When the user pops back to them, their cached width/height are stale. Today the placeholder stubs do not care, but the design is meant to outlive them (Profiles, Edit Profile, …).

```go
// internal/tui/shell/shell.go:56-61
case tea.WindowSizeMsg:
    m.width, m.height = msg.Width, msg.Height
    top := len(m.stack) - 1
    s, cmd := m.stack[top].Update(msg) // only the top sees this
    m.stack[top] = s
    return m, cmd
```

- [x] Iterate `m.stack` on `WindowSizeMsg` and call `Update(msg)` on every screen; batch the returned cmds.
- [ ] Document the constraint on `Screen` explicitly ("screens receive `WindowSizeMsg` only while on top") and have screens re-fetch on push.
- [ ] Add a test: push A → B, resize, pop B, assert A received the resize.

## Popped screen kept alive in the stack's backing array

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) (idiomatic Go: shrink-without-zero leaks references)

`m.stack` is `[]Screen` (interface slice). Re-slicing leaves the popped screen in the backing array, holding live references to whatever the screen captured. For a long-running TUI that pushes/pops hundreds of edit cycles, those screens never become collectible until the slice is reallocated by a deeper push.

```go
// internal/tui/shell/shell.go:81-86
case PopScreenMsg:
    if len(m.stack) > 1 {
        m.stack = m.stack[:len(m.stack)-1] // backing array still references the popped screen
    }
    return m, nil
```

- [x] Zero the slot before shrinking: `m.stack[len(m.stack)-1] = nil; m.stack = m.stack[:len(m.stack)-1]`.

## Three global-key stubs duplicate identical `esc → pop` logic

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) §"Code Smells" (needless repetition)

`notificationsStub`, `settingsStub`, and `infoStub` repeat the same 4-line `Update` that pops on `esc`. The only differences are `Title()` and `Body()` strings — pure data. Three near-identical structs is a smell for what is really one concept ("placeholder screen").

```go
// internal/tui/shell/welcome_stub.go:28-33  (same shape at :46-51, :62-67)
func (s notificationsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
    if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "esc" {
        return s, popCmd()
    }
    return s, nil
}
```

- [ ] Collapse the three into one `placeholderStub struct { title, body string }` with one `Update`; construct via `newNotificationsStub() = placeholderStub{title:"Notifications", body:"..."}`.
- [x] Keep three distinct types so they delete cleanly one task at a time (current layout).

## `welcome_stub.go` filename advertises only one of its four stubs

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) §"Source Structure" (filename should reflect contents)
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) (Common Closure Principle: code that changes together stays together; tasks 0023 and 0024 each delete different stubs from this same file, guaranteeing a merge conflict)

```go
// internal/tui/shell/welcome_stub.go — declares four unrelated stub types:
//   welcomeStub        (replaced by task 0024)
//   notificationsStub  (replaced by task 0023)
//   settingsStub       (replaced by task 0024)
//   infoStub           (replaced by task 0023)
```

- [ ] Split into `welcome_stub.go` (only `welcomeStub`) and `stubs.go` (the three global-key destination stubs).
- [ ] Rename the file to `stubs.go` and merge `welcome_stub_test.go` into `stubs_test.go`.
- [x] One file per stub (`notifications_stub.go`, etc.) matching the per-screen layout the real screens will adopt in 0023/0024.

## Stub types use value receivers despite the loop pattern relying on returned state

> [!WARNING]
>
> - [docs/guidelines/charm.md](../../../docs/guidelines/charm.md) §"Composing Sub-Models" (the loop `m.left, cmd = m.left.Update(msg)` relies on the returned value reflecting state changes; a value-receiver empty struct works only because there is no state)

The shell does `s, cmd := m.stack[top].Update(msg); m.stack[top] = s`. With an empty-struct value receiver, returned `s` is a zero copy — assigning it back works only because the type carries no state. The replacements in tasks 0023/0024 will have state (lists, selection, viewport); pointer receivers from the start force the next author into the correct pattern.

```go
// internal/tui/shell/welcome_stub.go:16  (same shape on all four stubs)
func (s welcomeStub) Update(_ tea.Msg) (Screen, tea.Cmd) { return s, nil }
```

- [x] Switch all four stubs to pointer receivers and have `new*Stub()` return `*welcomeStub` etc.
- [ ] Leave value receivers and add a comment on each stub: "Stub types are stateless. The real screen in task 002X must use a pointer receiver."

## `Screen.StatusKeys` is dead weight on every shipped screen

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) Interface Segregation Principle ("Callers should depend only on the behavior they actually use. Keep interfaces small")

Every concrete `Screen` returns `nil` from `StatusKeys`, and the test fakes implement it as boilerplate. The doc comment on `Screen` even admits the method is for "typical" screens — meaning the contract is already known to be unevenly used.

```go
// internal/tui/shell/welcome_stub.go — all four implementations
func (welcomeStub) StatusKeys() []key.Binding            { return nil }
func (notificationsStub) StatusKeys() []key.Binding      { return nil }
func (settingsStub) StatusKeys() []key.Binding           { return nil }
func (infoStub) StatusKeys() []key.Binding               { return nil }
// + recordingScreen, sizingSpy in shell_test.go also boilerplate
```

- [ ] Drop `StatusKeys` from `Screen`; introduce a narrow optional interface `type DynamicStatusKeys interface{ StatusKeys() []key.Binding }`. In `Model.View`, type-assert the top screen against it; fall back to nil when absent.
- [x] Keep as is; the surface is tiny and every real screen in 0024+ will implement it.

## `shell.Model.Update` mixes routing, global-key policy, notification fan-out, and toast plumbing in one switch

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) Single Responsibility Principle ("Keep unrelated reasons to change apart")

`Update` changes for four independent reasons: window resizing, global key policy, notification → log+toast fan-out, screen-stack mutation, plus the default toast/screen broadcast. The "toast is fed on every default message" path leaks knowledge of `Toast`'s unexported `expireMsg`. When a future screen needs to influence log behavior (dedup, severity filter), the edit lands in the shell, not in `notifications`.

```go
// internal/tui/shell/shell.go:71-86
case notifications.NotificationMsg:
    m.log.Add(msg.Notification)
    t, cmd := m.toast.Update(notifications.PushMsg{Notification: msg.Notification})
    m.toast = t
    return m, cmd

case PushScreenMsg:
    m.stack = append(m.stack, msg.Screen)
    return m, msg.Screen.Init()

case PopScreenMsg:
    if len(m.stack) > 1 {
        m.stack = m.stack[:len(m.stack)-1]
    }
    return m, nil
```

- [x] Extract small helpers on `Model` (`pushScreen`, `popScreen`, `notify`); `Update` reads as routing only.
- [ ] Keep inline (the switch is small enough today) and revisit when the second non-trivial concern is added.

## `Toast` and `Log` are held as concretes despite a narrow contract

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) Dependency Inversion Principle ("When a stable rule needs a side effect, pass the side effect in as a narrow interface or function owned by the caller")

`Model` calls only `log.Add`, `toast.Update`, `toast.View`, `toast.Empty`. Tests already feel the friction: `TestUpdate_NotificationMsgFeedsLogAndToast` reaches into `toast.Empty()` and `log.Entries()` as observability hooks rather than substituting fakes.

```go
// internal/tui/shell/shell.go:17-25
type Model struct {
    actions *actions.Actions
    toast   *notifications.Toast
    log     *notifications.Log
    ...
}
```

- [x] Declare consumer-side interfaces in `shell/` (`notifier`, `toaster`) and store those on `Model`; keep `New` accepting concretes.
- [ ] Leave as concretes — the interfaces would be speculative until a second toast implementation exists.

## `Model.actions` stored but never used

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) §"Code Smells" (speculative state / dead wiring)

The field, the constructor parameter, the nil-check panic, and the dedicated `TestNew_PanicsOnNilActions` exist only to validate wiring no current code path exercises. The doc justifies it as "for future screens".

```go
// internal/tui/shell/shell.go:17-25
type Model struct {
    actions *actions.Actions // never read in this package
    ...
```

- [ ] Remove the field, parameter, panic, and matching test. Reintroduce when the first screen that needs it lands in 0024+.
- [x] Keep it; the next task adds the first consumer and removing now means churn in `cmd/af/main.go` twice.

## `popCmd` / `pushCmd` live in `keys.go` but only one is used by global-key handling

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) §"Source Structure" (keep related code close together)

`pushCmd` is used only by `handleGlobalKey` (keys.go local); `popCmd` is used only by the three stubs in `welcome_stub.go`. Putting both in `keys.go` muddles the file's role with command construction.

```go
// internal/tui/shell/keys.go:68-74
func pushCmd(s Screen) tea.Cmd {
    return func() tea.Msg { return PushScreenMsg{Screen: s} }
}

func popCmd() tea.Cmd {
    return func() tea.Msg { return PopScreenMsg{} }
}
```

- [x] Move both next to the message types in `screen.go`.
- [ ] Inline at call sites (one-liner closures); drop the helpers.

## WHAT comment on the default-branch toast routing

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) §"Comments" ("Do not repeat what the next line of code already says")

The comment restates what the code does. The interesting WHY ("Toast harmlessly ignores foreign messages because `expireMsg` is unexported") is in the changelog/ADR but not in the source — and that's the part future readers actually need.

```go
// internal/tui/shell/shell.go:88-91
// Default routing: feed the toast (it ignores anything that
// isn't PushMsg/expireMsg) and forward to the active screen.
t, tCmd := m.toast.Update(msg)
m.toast = t
```

- [x] Replace with a WHY: `// Toast.Update has to run on every unhandled message because its internal expireMsg is unexported — we can't type-switch on it here. Toast ignores foreign messages, so this is safe.`
- [ ] Drop the comment entirely; the line below is self-explanatory.

## Magic key code `27` in stub test

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) §"Naming" (replace magic numbers and strings with named constants)

Every other test in the package identifies keys via `Code: 'n', Text: "n"`. Only this test uses raw byte `27` with a trailing comment.

```go
// internal/tui/shell/welcome_stub_test.go:24
_, cmd := s.Update(tea.KeyPressMsg{Code: 27, Text: ""}) // esc
```

- [x] Replace with the bubbletea named code (e.g. `tea.KeyEscape` if present) or `tea.KeyPressMsg{Code: 0x1b, Text: ""}` — verify the v2 API for the canonical form.

## Missing doc comments on `Model.Init` / `Update` / `View`

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) (godoc conventions; exported symbols carry doc comments)

`Model` has a doc comment; its three methods (which form the `tea.Model` contract this package fulfils) do not. `Screen`'s interface methods are documented on the interface; `Model` deserves the same on its method set.

```go
// internal/tui/shell/shell.go:49, :53, :100
func (m Model) Init() tea.Cmd { ... }
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { ... }
func (m Model) View() tea.View { ... }
```

- [x] Add one-liners on each method describing its contract.

## Building-block diagram declares arrows the shell does not import

> [!WARNING]
>
> - [docs/guidelines/documentation.md](../../../docs/guidelines/documentation.md) ("document current reality, not planned state")

The updated Level-1 diagram and the plan both list `tui_shell --> tui_components_modal` and `tui_shell --> tui_mnemonic`, but `internal/tui/shell/*.go` imports neither. The shell uses raw `bubbles/key.Binding` values; modal/mnemonic usage will come from individual screens in 0024+, not from the shell itself.

```text
// docs/architecture/05-building-block-view.md:13 and :31
tui_shell --> tui_mnemonic["tui/components/mnemonic"]
...
tui_shell --> tui_components_modal["tui/components/modal"]
```

- [ ] Remove the two arrows; drop `tui_components_modal` from the leaf-class line if no other node references it.
- [x] Leave the arrows and add a note that they document future shell→component coupling.

## Building-block prose claims "nothing else imports `app`", but `cmd/af` does

> [!WARNING]
>
> - [docs/guidelines/documentation.md](../../../docs/guidelines/documentation.md) ("document current reality, not planned state")

The updated prose says "The TUI layer reaches `app` only through the `actions` adapter; nothing else imports `app`." `cmd/af/main.go` imports `app` and `registry` at the composition root — which is standard practice, but contradicts the literal statement and the diagram (no `cmdaf → app` arrow).

```go
// cmd/af/main.go:11-16
import (
    "github.com/hexworks/agentfiles/internal/actions"
    "github.com/hexworks/agentfiles/internal/app"
    "github.com/hexworks/agentfiles/internal/registry"
    ...
)
```

- [x] Revise the prose: "the TUI layer reaches `app` only through `actions`; the binary at `cmd/af` wires both as the composition root." Add `cmdaf --> app` and `cmdaf --> registry` arrows to the diagram.
- [ ] Hide the wiring behind a small `wiring` package so the literal statement holds.

## Tests instantiate a real `app.Service` despite shell having no `app` dependency

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) §"Start With The Smallest Useful Test"

`newTestShell` pulls a tempdir, builds an `*app.Service`, and constructs `*actions.Actions` whose methods are never invoked by any shell test. If `actions.New` ever validates the registry path, every shell test breaks for unrelated reasons.

```go
// internal/tui/shell/shell_test.go:17-22
func newTestShell(t *testing.T) Model {
    dir := t.TempDir()
    svc := app.New(dir + "/registry.json")
    return New(actions.New(svc), notifications.NewLog())
}
```

- [ ] Drop `actions` from `Model` (see related finding above), removing the `app` dependency from tests.
- [ ] Add a zero-value `actions.NewForTest()` (or accept nil) so tests construct an empty `Actions` without `app`.
- [x] Leave the wiring; it works today and re-aligning when the actions field is actually used is a smaller diff.

## `TestRenderStatusBar_IncludesGlobalAndDynamic` mixes two unrelated behaviors

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) §"Test One Behavior At A Time"

The test asserts the positive ("global + dynamic appear") with one call, then resets and asserts the negative ("screen-level verbs do not leak") with another. Two setups, two behaviors, one test. The negative half also tests for verbs the producer never injects, so it would pass even if composition were broken differently.

```go
// internal/tui/shell/shell_test.go:236-268
got := renderStatusBar(global, []key.Binding{edit, del})
... // positive substring asserts
emptyBar := renderStatusBar(global, nil)
... // negative substring asserts
```

- [x] Split into `TestRenderStatusBar_IncludesGlobalAndDynamic` and `TestRenderStatusBar_OmitsScreenLevelVerbsWhenDynamicEmpty`.
- [ ] Delete the negative half — the rule belongs in a screen-level test of a real screen.

## `PushScreenMsg` `Init()` cmd contract is not asserted

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) §"Test One Behavior At A Time"

`TestUpdate_PushScreenMsgGrowsStack` ignores the returned cmd. The plan (Step 6, §Update routing order, point 4) says "append to stack, run new screen's `Init()`". If somebody changes `return m, msg.Screen.Init()` to `return m, nil`, every existing test still passes.

```go
// internal/tui/shell/shell.go:77-79
case PushScreenMsg:
    m.stack = append(m.stack, msg.Screen)
    return m, msg.Screen.Init() // contract is untested
```

- [x] Extend the push test using a screen whose `Init()` returns a sentinel cmd; assert the cmd matches.
- [ ] Add a separate `TestUpdate_PushScreenMsgRunsNewScreenInit`.

## Default-routing branch untested for non-key messages

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) §"Test One Behavior At A Time"

`TestUpdate_NonGlobalKeyReachesScreen` exercises a key that falls through to default routing. The "default routing batches toast + screen cmds" promise for arbitrary messages (custom ticks, timer messages, etc.) has no dedicated coverage. If somebody removes the toast update from the default branch, no test fails.

```go
// internal/tui/shell/shell.go:88-97 — the default branch
t, tCmd := m.toast.Update(msg)
m.toast = t
top := len(m.stack) - 1
s, sCmd := m.stack[top].Update(msg)
m.stack[top] = s
return m, tea.Batch(tCmd, sCmd)
```

- [x] Add `TestUpdate_DefaultRoutingFeedsToastAndScreen` sending a synthetic `tea.Msg` (not key, not WindowSize, not Notification, not Push/Pop) and asserting both the recording screen received it and a non-nil batched cmd is returned.

## `TestUpdate_NotificationMsgFeedsLogAndToast` chains three assertions

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) §"Test One Behavior At A Time"

```go
// internal/tui/shell/shell_test.go:175-197
if len(entries) != 1 { ... }
if entries[0].Text != "profile created" { ... }     // duplicates Log.Add semantics
if m.toast.Empty() { ... }
```

The text equality duplicates `notifications/log` tests; the load-bearing claim here is "log received one entry" and "toast became non-empty".

- [ ] Drop the `entries[0].Text` assertion.
- [x] Split into `_FeedsLog` and `_FeedsToast`.

## `View()` body-height arithmetic is untested

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) §"Start With The Smallest Useful Test"

The only `View` test substring-checks the output. The body-height computation — including the `bodyH < 0` clamp and the toast-empty-vs-present branch — is not verified. A regression that hands the wrong height to `Body` will silently truncate or expand content but no test catches it.

```go
// internal/tui/shell/shell.go:107-117
headerH := lipgloss.Height(title)
toastH := 0
if toast != "" { toastH = lipgloss.Height(toast) }
bodyH := m.height - headerH - toastH - lipgloss.Height(status)
if bodyH < 0 { bodyH = 0 }
body := top.Body(m.width, bodyH)
```

- [x] Add a screen whose `Body` records `(width, height)`; assert (a) on `WindowSizeMsg{80, 24}` with empty toast, height equals `24 - headerH - 1`; (b) on tiny `Height: 2`, body height clamps to 0; (c) after a NotificationMsg, body height shrinks by the toast height.
