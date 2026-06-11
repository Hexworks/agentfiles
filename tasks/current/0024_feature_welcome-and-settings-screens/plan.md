# Task 0024 — Welcome + Settings screens

Links: [description.md](./description.md) · parent refactor architecture in ADR [`docs/adr/0011-tui-screen-router.md`](../../../docs/adr/0011-tui-screen-router.md).

## Context

Task 0021 stood up the alt-screen Bubble Tea shell with a screen router, global key bindings, and two intentional placeholders: `welcomeStub` (root screen) and `settingsStub` (target of the global `s` key). Task 0024 replaces both with the real Welcome and Settings screens so the top-level navigation tree is wired before the entity screens land in 0025–0029.

The Welcome screen owns the menu the rest of the app branches from (Profiles / Settings / Quit). The Settings screen is intentionally an MVP stub — a "Coming soon." body plus the `[Back]` mnemonic button — but it ships the real screen type, so future work on Settings won't touch shell wiring.

## What changes

### New files

1. `internal/tui/shell/welcome.go` — `welcomeScreen` (real Welcome).
2. `internal/tui/shell/welcome_test.go` — navigation + dispatch tests.
3. `internal/tui/shell/settings.go` — `settingsScreen` (real Settings).
4. `internal/tui/shell/settings_test.go` — back-button + esc + status-key tests.
5. `internal/tui/shell/profiles_stub.go` — `profilesStub` placeholder for task 0025.
6. `internal/tui/shell/profiles_stub_test.go` — minimal title/body/esc test.

### Files to delete

- `internal/tui/shell/welcome_stub.go`
- `internal/tui/shell/welcome_stub_test.go`
- `internal/tui/shell/settings_stub.go`
- Settings-stub tests inside `internal/tui/shell/stubs_test.go` (`TestSettingsStub_*`) — folded into `settings_test.go`.

### Files to modify

- `internal/tui/shell/shell.go` — change `stack: []Screen{newWelcomeStub()}` → `stack: []Screen{newWelcomeScreen()}` at `shell.go:69`.
- `internal/tui/shell/keys.go` — change `pushCmd(newSettingsStub())` → `pushCmd(newSettingsScreen())` at `keys.go:62`.
- `internal/tui/shell/shell_test.go` — anywhere it asserts the initial stack screen type is `welcomeStub` or pushes `settingsStub` for the push/pop tests, switch to the new types (or to a local test screen so the tests stay decoupled from the real ones).

## Design

### welcomeScreen

```go
type welcomeItem struct {
    label  string
    action func() tea.Cmd
}

type welcomeScreen struct {
    cursor int
    items  []welcomeItem
}

func newWelcomeScreen() *welcomeScreen { ... }
```

Items in fixed order:

| Index | Label    | Action                            |
| ----- | -------- | --------------------------------- |
| 0     | Profiles | `pushCmd(newProfilesStub())`      |
| 1     | Settings | `pushCmd(newSettingsScreen())`    |
| 2     | Quit     | `tea.Quit`                        |

`Update`:

- `tea.KeyPressMsg`:
  - `up` / `k` → `cursor = (cursor - 1 + len(items)) % len(items)`
  - `down` / `j` → `cursor = (cursor + 1) % len(items)`
  - `enter` / `v` → return `items[cursor].action()`
- everything else → no-op.

Wrap-around keeps navigation predictable for a 3-item list. Matching uses local `key.NewBinding(...)` so the screen stays self-contained (the shell's `globalKeyMap.Up` / `Down` are display-only).

`Title() = "Agentfiles"` (per the ASCII mockup heading).

`Body(width, height)`:

```
┃ Choose a task
┃ > Profiles
┃   Settings
┃   Quit
```

`lipgloss.JoinVertical` over a small loop. Selected row prefixed with `> `, others with `  `. Vertical bar prefix uses a muted style. The `> ` glyph is the primary selection signal (not color).

`StatusKeys() = nil` — no mnemonic buttons on Welcome.

### settingsScreen

```go
type settingsScreen struct {
    back *mnemonic.Button
}

func newSettingsScreen() *settingsScreen {
    return &settingsScreen{
        back: mnemonic.New("Back", 'b', func() tea.Cmd { return popCmd() }),
    }
}
```

`Update`:

- `tea.KeyPressMsg`:
  - `s.back.Matches(kp)` → return `s.back.Trigger()` (= `popCmd()`).
  - `kp.Code == tea.KeyEsc` → `popCmd()`.
- everything else → no-op.

`Title() = "Settings"`.

`Body(width, height)`:

```
 Coming soon.

                                                                                       [Back]
```

Two-line composition. "Coming soon." rendered at the top; `s.back.View()` rendered right-aligned on a lower row using `lipgloss.Place(width, ..., lipgloss.Right, lipgloss.Bottom)`. If height is small (< 3), drop the spacing and stack vertically — the shell already clamps `bodyH` to 0 when overdrawn.

`StatusKeys() = []key.Binding{s.back.Binding()}` — Back is explicitly called out in the task description as belonging in the status bar.

### profilesStub

Mirror of the old `welcomeStub` / `settingsStub` pattern.

- `Title() = "Profiles"`
- `Body() = "(profiles screen — task 0025)"`
- `Update`: pop on `tea.KeyEsc`. (Lets the user back out instead of being stuck.)
- `StatusKeys() = nil`.

This stub is overwritten by task 0025 — keep it minimal.

## Tests

All under `internal/tui/shell/`. Patterns mirror existing `stubs_test.go` / `welcome_stub_test.go`.

### welcome_test.go

- `TestWelcomeScreen_InitialCursor` — fresh screen, cursor at 0 (Profiles).
- `TestWelcomeScreen_DownArrowMovesCursor` — down moves cursor 0 → 1; `j` same.
- `TestWelcomeScreen_UpArrowWrapsFromTop` — up from cursor 0 wraps to 2 (Quit); `k` same.
- `TestWelcomeScreen_DownArrowWrapsFromBottom` — down from cursor 2 wraps to 0.
- `TestWelcomeScreen_EnterOnProfilesPushesProfilesStub` — enter at cursor 0 emits `PushScreenMsg{Screen: *profilesStub}`.
- `TestWelcomeScreen_VOnSettingsPushesSettingsScreen` — set cursor 1, `v` emits `PushScreenMsg{Screen: *settingsScreen}`.
- `TestWelcomeScreen_EnterOnQuitEmitsQuit` — set cursor 2, enter returns a cmd whose `tea.Msg` is `tea.QuitMsg{}`.
- `TestWelcomeScreen_BodyContainsAllItems` — body string contains "Profiles", "Settings", "Quit", and the `> ` marker on the selected row.
- `TestWelcomeScreen_TitleIsAgentfiles`.

### settings_test.go

- `TestSettingsScreen_BTriggersPop` — `b` key emits `PopScreenMsg`.
- `TestSettingsScreen_EscTriggersPop` — `esc` emits `PopScreenMsg`.
- `TestSettingsScreen_OtherKeysDoNothing` — `x` returns nil cmd.
- `TestSettingsScreen_StatusKeysExposesBack` — `StatusKeys()` contains one binding whose `Help().Key == "b"` and `Help().Desc == "Back"`.
- `TestSettingsScreen_TitleAndBody` — title `"Settings"`, body contains `"Coming soon"` and `"[Back]"` (label render through mnemonic.Button).

### profiles_stub_test.go

- `TestProfilesStub_EscEmitsPopCmd` — same shape as the deleted `TestSettingsStub_EscEmitsPopCmd`.
- `TestProfilesStub_TitleAndBody` — `"Profiles"` title, body contains "task 0025".

### Shell tests

- `internal/tui/shell/shell_test.go`: any test that names `welcomeStub` directly (initial-stack-depth/type checks) updates to `welcomeScreen`. Push/pop mechanics tests should switch to a local test screen (e.g. the existing `recordingScreen`) so they don't depend on the real screens; if the existing tests already use `settingsStub` as the "second screen", switch that to a local stub too.
- `internal/tui/shell/stubs_test.go`: remove `TestSettingsStub_EscEmitsPopCmd` and `TestSettingsStub_TitleAndBody` — they now live in `settings_test.go` against the real screen.

## Step-by-step execution

1. Add `profiles_stub.go` + `profiles_stub_test.go`. Smallest unit; lets later code reference `newProfilesStub()`.
2. Add `welcome.go` + `welcome_test.go`. Replaces `welcomeStub`.
3. Add `settings.go` + `settings_test.go`. Replaces `settingsStub`.
4. Update `shell.go:69` (initial stack) and `keys.go:62` (`s` global) to point at the real constructors.
5. Update `shell_test.go` so push/pop and initial-stack assertions reference `welcomeScreen` (or a local test stub) instead of `welcomeStub` / `settingsStub`.
6. Delete `welcome_stub.go`, `welcome_stub_test.go`, `settings_stub.go`. Remove `TestSettingsStub_*` from `stubs_test.go`.
7. `make fmt && make lint && make test && make build` — green gate.
8. Manual: `./bin/af` → lands on Welcome menu; arrow + `j`/`k` move selection; `enter` and `v` choose; `Settings` shows "Coming soon" + `[Back]`; `b` and `esc` return; `s` from anywhere also reaches Settings.

## Guideline conformance

- **TUI / Clean Architecture**: screens are pure UI state; no domain calls. Welcome only dispatches `PushScreenMsg` / `tea.Quit`; Settings only dispatches `PopScreenMsg`. No imports outside `internal/tui/...` and `charm.land/*`.
- **Clean code / SOLID**: each screen one responsibility (menu vs. coming-soon-stub-with-back). `welcomeItem` is a tiny struct over `map[string]func()` (per Go guideline: explicit structs over maps). No business logic leaks into screens.
- **Domain model**: untouched. No new domain types.
- **Testing**: every observable behavior — cursor movement, dispatch, back, status keys — has a focused unit test. Tests use direct `Update(tea.KeyPressMsg{...})` invocation; no terminal needed.

No ADR change (the architecture is already in `0011-tui-screen-router.md`). No guideline change. No `docs/architecture/` change. The changelog under `docs/changelog/` will record this task.

## Verification

```bash
make fmt
make lint
make test
make build
./bin/af
```

Manual pass:

1. App opens on Welcome. Cursor on Profiles.
2. `j` / `down` moves to Settings, then Quit, wraps to Profiles.
3. `k` / `up` wraps the other direction.
4. `enter` on Profiles → Profiles stub. `esc` returns.
5. `enter` on Settings → Settings screen. `b` returns. Re-enter; `esc` also returns.
6. From any screen, `s` → Settings (global key still works).
7. `enter` on Quit → app exits cleanly.
