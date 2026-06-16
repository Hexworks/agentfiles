# Palette-Driven Theming With External Configuration

## Status

accepted

## Context

The TUI's colors and styles lived in `internal/tui/styles/styles.go` as
package-init `var`s built from hard-coded ANSI indices
(`lipgloss.Color("8")`), with additional lipgloss styles constructed inline
inside individual components (help drew its own border, treetable called
`table.DefaultStyles()`, the shell kept a private `titleStyle`). Three
problems followed:

1. A single visual change — say, the muted color — meant editing several
   files, because the same shade was re-derived at each call site.
2. Styles were frozen at package init, so there was no point at which an
   alternate color set could be installed.
3. There was no user-facing customization surface; the palette was baked
   into the binary.

The notifications work (ADR 0007) had already established `tui/styles` as
the shared styles vocabulary, so the natural move was to deepen that
package rather than spread theming logic outward.

## Decision

Theming is centralized behind a **palette** and a single rebuild function.

- `palette.go` defines `type Palette struct` with semantic `color.Color`
  roles (`Text`, `Muted`, `Highlight`, `Cyan`, `Red`, `Green`, `Yellow`,
  `Magenta`, `MnemonicHL`) — roles, not shades — plus `DefaultPalette()`
  holding the original ANSI indices.
- `styles.go` no longer freezes styles at init. Its exported style `var`s
  are rebuilt by `func Apply(p Palette)`, which is the single edit-point
  for a palette swap. `init()` calls `Apply(DefaultPalette())`; `Current()`
  returns the active palette.
- `huh.go` (`HuhTheme()`) and `table.go` (`TableStyles()`) derive the form
  theme and the shared bubbles-table style from the active palette so
  forms and tables theme identically to everything else.
- `config.go` adds an **optional** external override. `LoadConfig(path)`
  reads a `theme.json`, merges its colors over the defaults via
  `mergePalette`, and calls `Apply`. `DefaultConfigPath()` resolves
  `$XDG_CONFIG_HOME/agentfiles/theme.json`. A missing file is a no-op; a
  malformed file returns a typed `ConfigReadError` / `ConfigParseError`.
- `cmd/af/main.go` parses a `--theme` flag (defaulting to
  `DefaultConfigPath()`) and calls `LoadConfig` before constructing the
  Bubble Tea program. A malformed theme aborts startup rather than falling
  back silently, so a typo surfaces immediately.

## Consequences

- A palette change is one edit to `Apply` (or one `theme.json`), not a
  sweep across components. Components read named style `var`s and stay
  ignorant of color values.
- Users gain a customization surface without a config file becoming
  mandatory; the XDG path keeps it where Linux tool config is expected.
- Startup now has a fail-fast branch for a bad theme file — a deliberate
  trade of "silent default" for "visible error".
- The styles package keeps its leaf status: nothing in `internal/`
  (domain) imports it, so theming remains a presentation concern with no
  reach into the domain model or glossary.

## References

- `internal/tui/styles/{palette,config,huh,table,styles}.go`
- `cmd/af/main.go` (`--theme` flag, `LoadConfig`)
- ADR 0007 (rendering belongs to the TUI; styles vocabulary lives in
  `tui/styles`)
