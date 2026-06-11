# 0024 changes

Replaces the `welcomeStub` and `settingsStub` placeholders installed by
task 0021 with real top-level screens. The Welcome screen is now a
vertical-list menu over `Profiles` / `Settings` / `Quit` with
arrow + `j`/`k` navigation, wrap-around, and `enter` / `v` to choose. The
Settings screen is an MVP body — a single `Coming soon.` line — but it
ships the real `settingsScreen` type wired to the `[Back]` mnemonic
button (`b` or `esc` returns to the parent). A small `profilesStub`
takes the slot the Welcome → Profiles route lands on until task 0025
lands the real screen.

The shell wiring (`shell.go` initial stack + `keys.go` global `s`
target) now points at the real screen constructors. Doc comments on
the `shell` package and the `Settings` global key binding were
refreshed so the "placeholder" language is no longer load-bearing.

## Decisions

- **Wrap-around cursor.** Up from `Profiles` jumps to `Quit`; down from
  `Quit` jumps to `Profiles`. **Why:** with a 3-item menu, clamping
  punishes the user for choosing the rightmost direction. Wrap matches
  the menu-skim pattern users already get from `bubbles/list`.
- **`tea.Quit` lives inside `welcomeItem.action`, not as a route.**
  **Why:** the menu is presentational; the action it produces is just a
  `tea.Cmd`. Treating "Quit" as a regular item keeps the cursor + render
  loop identical for every row instead of bolting on a special-case
  branch.
- **`StatusKeys()` returns the `[Back]` binding even though Back is
  visible in the body.** **Why:** task description treats `[Back]` as
  the documented exception to the "screen-level buttons not duplicated"
  rule, and on an otherwise empty Settings body the status bar is the
  only place a user discovers `b`.
- **A separate `profilesStub`, not a reuse of `notificationsScreen` or
  similar.** **Why:** stubs are intentionally minimal so task 0025
  doesn't have to disentangle inherited behavior from the placeholder.

Considered and rejected:

- **Putting the Welcome cursor logic behind `bubbles/list`.** Too much
  configuration surface for a fixed three-item menu, and `bubbles/list`
  brings its own filter UI / status line that doesn't fit the
  bar-prefixed mockup.

## Assumptions

- **Welcome `Title()` is the literal `Agentfiles`.** **Why:** the task's
  ASCII mockup names the banner that way. The previous "Welcome" title
  inside `welcomeStub` was a placeholder.

## Other Notes

- `internal/tui/shell/shell_test.go` keeps using the real
  `settingsScreen` for push/pop mechanics tests — type dedup behavior is
  exercised end-to-end without a synthetic stub.
- `internal/tui/shell/stubs_test.go` lost its `tea` import after the
  settings-stub assertions moved to `settings_test.go`.

## welcomeScreen

A vertical-list root screen with a fixed route table. The cursor
wraps; `enter` / `v` fire the selected item's action; everything else
is a no-op.

```go
// before — placeholder root, no menu
type welcomeStub struct{}

func (s *welcomeStub) Init() tea.Cmd                      { return nil }
func (s *welcomeStub) Update(_ tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (s *welcomeStub) Body(_ int, _ int) string           { return "agentfiles — press q to quit" }
func (s *welcomeStub) Title() string                      { return "Welcome" }
func (s *welcomeStub) StatusKeys() []key.Binding          { return nil }
```

```go
// after — real Welcome menu with cursor + key.Bindings
type welcomeItem struct {
    label  string
    action func() tea.Cmd
}

type welcomeScreen struct {
    cursor int
    items  []welcomeItem

    up, down, choose key.Binding
}

func newWelcomeScreen() *welcomeScreen {
    return &welcomeScreen{
        items: []welcomeItem{
            {label: "Profiles", action: func() tea.Cmd { return pushCmd(newProfilesStub()) }},
            {label: "Settings", action: func() tea.Cmd { return pushCmd(newSettingsScreen()) }},
            {label: "Quit",     action: func() tea.Cmd { return tea.Quit }},
        },
        up:     key.NewBinding(key.WithKeys("up", "k")),
        down:   key.NewBinding(key.WithKeys("down", "j")),
        choose: key.NewBinding(key.WithKeys("enter", "v")),
    }
}
```

## settingsScreen

The placeholder Settings stub becomes the real screen type. It owns a
`mnemonic.Button` for `[Back]`, dispatches `popCmd()` on `b` or `esc`,
and advertises the `[Back]` binding in `StatusKeys()` so the bar shows
`b back` while the screen body is empty.

```go
// before — Esc-only placeholder, no Back button, no status hint
type settingsStub struct{}

func (s *settingsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
    if kp, ok := msg.(tea.KeyPressMsg); ok && kp.Code == tea.KeyEsc {
        return s, popCmd()
    }
    return s, nil
}
```

```go
// after — real Settings screen with Back mnemonic, b and esc both pop,
// and StatusKeys() advertises the Back binding to the status bar.
type settingsScreen struct {
    back *mnemonic.Button
}

func newSettingsScreen() *settingsScreen {
    return &settingsScreen{
        back: mnemonic.New("Back", 'b', func() tea.Cmd { return popCmd() }),
    }
}

func (s *settingsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
    kp, ok := msg.(tea.KeyPressMsg)
    if !ok {
        return s, nil
    }
    if s.back.Matches(kp) {
        return s, s.back.Trigger()
    }
    if kp.Code == tea.KeyEsc {
        return s, popCmd()
    }
    return s, nil
}

func (s *settingsScreen) StatusKeys() []key.Binding {
    return []key.Binding{s.back.Binding()}
}
```

## profilesStub

A minimal stand-in for the real Profiles screen task 0025 will write.
Pop-on-Esc behavior matches the deleted `settingsStub`; everything
else is intentionally absent.

```go
// after — placeholder for task 0025
type profilesStub struct{}

func (s *profilesStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
    if kp, ok := msg.(tea.KeyPressMsg); ok && kp.Code == tea.KeyEsc {
        return s, popCmd()
    }
    return s, nil
}

func (s *profilesStub) Body(_ int, _ int) string  { return "(profiles screen — task 0025)" }
func (s *profilesStub) Title() string             { return "Profiles" }
func (s *profilesStub) StatusKeys() []key.Binding { return nil }
```

## Shell wiring

The root stack seed and the `s` global-key target now reference the
real screens. The Welcome screen body content changes the visible
title from `Welcome` to `Agentfiles`, so the view assertion in
`shell_test.go` is updated to match.

```go
// before
stack:   []Screen{newWelcomeStub()},
...
case key.Matches(kp, m.keys.Settings):
    return pushCmd(newSettingsStub()), true
```

```go
// after — real screens replace the placeholders.
stack:   []Screen{newWelcomeScreen()},
...
case key.Matches(kp, m.keys.Settings):
    return pushCmd(newSettingsScreen()), true
```
