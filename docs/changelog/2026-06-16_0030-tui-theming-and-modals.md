# 0030 changes

A batch of TUI-presentation work landed across nineteen commits between
`9751add` and HEAD without per-task changelogs. This summary reconciles
them. The work splits into six themes: a palette-driven theming subsystem
loadable from an external `theme.json`; a `script/` folder running
single-file Go programs via `gorun`; SGR-aware mnemonic buttons plus the
removal of number-jump navigation; extraction of a reusable `panel`
component and modal `WithCaption`; help and notifications moving from
router-stack screens to shell-owned modals; and a per-screen description
row under the title.

## Decisions

- **Centralize styles behind `styles.Apply(Palette)` instead of frozen
  package-init `var`s.** — **Why:** colors were hard-coded ANSI indices
  scattered across components, so a theme change meant editing every call
  site. A single `Apply` rebuild point makes the palette swappable at
  runtime and keeps the styles vocabulary in one package. Promoted to
  ADR 0012.
- **Load the active theme from an external `theme.json`, missing-file is a
  no-op.** — **Why:** users get a customization surface without a config
  file being mandatory; a malformed file is a hard startup error
  (`ConfigParseError`) rather than silent fallback, so a typo is visible.
  See ADR 0012.
- **Help and notifications are shell-owned modal overlays, not screens on
  the router stack.** — **Why:** a stack screen consumes the global key
  set, so opening help would shadow the `q` / `ctrl+c` quit binding.
  Keeping the dialog off the stack lets the global quit stay live while it
  is open. Promoted to ADR 0013.
- **Add project scripts as single-file Go programs run by `gorun`.** —
  **Why:** scripting in the project's own language beats a second
  (shell/Python) toolchain; `script/setup` bootstraps the toolchain
  idempotently so the shebang works on a fresh machine. Promoted to
  ADR 0014.
- **Remove number-jump mnemonic navigation; keep single-letter labelled
  buttons.** — **Why:** the modifier-keyed number index (`AddMnemonic`,
  `Modifier`, `parseMnemonicPress`) duplicated what Tab/Shift+Tab already
  did and complicated `focus`. Single-letter button mnemonics — the part
  users actually pressed — stay.
- **Left-align button rows.** — **Why:** right-`PlaceHorizontal` buttons
  drifted with terminal width; a leading-space left alignment keeps them
  anchored and consistent across screens.

## Assumptions

- **The `theme.json` location follows XDG.** — **Why:** `DefaultConfigPath`
  resolves `$XDG_CONFIG_HOME/agentfiles/theme.json`, matching where the
  rest of a Linux user's tool config lives; no separate config-dir flag was
  warranted beyond the existing `--theme` override.
- **Theming is presentation, not domain.** — **Why:** the palette touches
  only `internal/tui`; no domain package imports it, so the glossary and
  domain model are unaffected. Recorded here so a later reader does not
  hunt for a domain concept that does not exist.

## Other Notes

- Architecture docs updated: `docs/architecture/05-building-block-view.md`
  rewrites the `tui/styles` and `tui/components/modal` entries and solidifies
  the previously-dotted `future` arrows; `08-concepts.md` gains a
  *Theming And Palette* concept. New ADRs 0012–0014 added to
  `docs/adr/README.md` and `09-architecture-decisions.md`.
- Manual pages for every screen were added under `docs/manual/` in the same
  batch (commit `cab46b4`); the stray `docs/manual/xul/something.md`
  placeholder was removed.
- Tests added alongside the code: `mnemonic/button_test.go` (SGR chunk
  rendering, selected view), updated `focus/help/treetable` tests, and
  `shell` stub/test updates for the description row and modal routing.

## Palette-driven theming subsystem

`internal/tui/styles` went from one file of frozen styles to a palette
engine: `palette.go` (`Palette` struct of semantic `color.Color` roles +
`DefaultPalette()`), `config.go` (`LoadConfig`, `DefaultConfigPath`,
typed `ConfigReadError` / `ConfigParseError`), `huh.go` (`HuhTheme()`),
`table.go` (`TableStyles()`), and a rewritten `styles.go` whose vars are
rebuilt by `Apply(Palette)`.

```go
// before — internal/tui/styles/styles.go
var (
    ColorMuted = lipgloss.Color("8")
    titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
    // …styles frozen at package init, colors as raw ANSI indices
)
```

```go
// after — palette roles rebuilt through Apply
var current Palette

func Apply(p Palette) {
    current = p
    TextStyle = lipgloss.NewStyle().Foreground(p.Text)
    ShellTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(p.Highlight)
    // …every exported style derived from the active palette
}

func init() { Apply(DefaultPalette()) }
func Current() Palette { return current }
```

```go
// after — cmd/af/main.go loads an optional external theme before run
themePath := flag.String("theme", styles.DefaultConfigPath(), "path to theme.json")
flag.Parse()
if err := styles.LoadConfig(*themePath); err != nil {
    fmt.Fprintln(os.Stderr, err) // malformed theme aborts startup
    os.Exit(1)
}
```

## Go single-file scripting

A new `script/` folder runs Go as a scripting language. `script/hello` is
a single-file Go program with a `gorun` shebang; `script/setup` is a POSIX
`sh` bootstrap that installs the Go toolchain (preferring `mise`) and
`go install github.com/erning/gorun@latest`.

```sh
# after — script/hello (excerpt)
#!/usr/bin/env gorun
package main

import "fmt"

func main() { fmt.Println("hello") }
```

## SGR-aware mnemonic buttons; number-jump navigation removed

`mnemonic.Button` dropped its `Bracket` / `Label` lipgloss styles for two
`ansi.Style` chunks so a button can render inside a selected table row
without emitting a mid-string `\x1b[0m` that would terminate the parent's
highlight. The number-jump index in `focus` was deleted entirely.

```go
// before — internal/tui/components/mnemonic/button.go
type Styles struct {
    Bracket lipgloss.Style
    Label   lipgloss.Style
}
// render() interpolated lipgloss styles, each ending in its own reset
```

```go
// after — SGR chunks, one trailing reset
type Styles struct {
    Mnemonic ansi.Style
    Text     ansi.Style
}
func (b *Button) ViewSelected() string { /* uses SelectedStyles() */ }
// render() writes one SGR per chunk transition + a single ansi.ResetStyle
```

```go
// before — internal/tui/components/focus/main.go
m.focus.AddMnemonic(m.files, '1', focus.WithModifier(focus.ModAlt))
```

```go
// after — Tab / Shift+Tab only
m.focus.Add(m.files)
```

## Reusable panel component and modal captions

The hand-drawn rounded frame with a caption embedded in the top border was
extracted from `treetable` into `panel.Render`; `modal` gained
`WithCaption` (drawing its frame through the panel) and `Active()`.

```go
// before — internal/tui/components/treetable/treetable.go
func (m *Model) renderPanel(body string) string {
    // bespoke border drawing + caption splicing, duplicated per widget
}
```

```go
// after — internal/tui/components/panel/panel.go
func Render(focused bool, caption, body string, st Styles) string { /* … */ }

// treetable and modal now both call panel.Render
return panel.Render(m.focused, m.caption, body, panel.DefaultStyles())
```

## Help and notifications as shell-owned modals

`infoScreen` and `notificationsScreen` (both router-stack `Screen`s) were
deleted; the shell now owns `helpModal` / `notificationsModal` overlays
opened by `ShowHelpMsg` / `ShowNotificationsMsg`, keeping the global quit
binding live while open.

```go
// before — internal/tui/shell/info.go (deleted)
type infoScreen struct{ backOnlyScreenBase }
// pushed onto the stack via pushCmd(newInfoScreen()), consuming global keys
```

```go
// after — internal/tui/shell/shell.go
type Model struct {
    // …
    helpModal          *modal.Modal
    notificationsModal *modal.Modal
}

func (m *Model) openHelp(topic string) { /* overlay, off the stack */ }
// routeHelpKey closes on esc only; q / ctrl+c still quit underneath
```

## Per-screen description row

The `Screen` interface gained `Description() string`; the shell renders a
muted-italic caption under the title, growing `chromeHeight` from 4 to 7.

```go
// before — internal/tui/shell/screen.go
type Screen interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    Body(width int) string
    Title() string
    StatusKeys() []key.Binding
}
```

```go
// after — adds Description, rendered between title and body
type Screen interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    Body(width int) string
    Title() string
    Description() string
    StatusKeys() []key.Binding
}
```
