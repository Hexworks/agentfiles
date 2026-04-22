# Remove CLI Subcommands In Favor Of Pure TUI

## Status

accepted (supersedes [0005](./0005-tui-as-the-only-user-interface.md))

## Context

ADR 0005 made the TUI the only interactive surface but kept a Cobra command
tree around so users could type `af profile create`, `af project apply`, and
so on to jump directly to the matching form. In practice this compromise
produced two parallel sources of truth for the available actions: the Cobra
subcommand set declared in `internal/appcmd/root.go` and the hardcoded menu
structure in `internal/tui/tui.go`. They dispatched to the same `tui.Run*`
functions, but their option lists were maintained by hand and nothing
enforced that they stayed in sync.

The duplication was not theoretical. A recent attempt to hide `asset init`
by commenting out its `AddCommand` call in `root.go` had no user-visible
effect because the TUI menu rebuilt the option independently and called the
flow directly. The Cobra tree turned out to only affect the shell-level
shortcut invocations; the main menu was unaware of it.

## Decision

Drop the Cobra command tree and with it the `internal/appcmd` package.
Running `af` now always opens the top-level menu; there are no subcommand
paths. The single remaining flag, `--registry`, is parsed with the standard
library `flag` package directly in `cmd/af/main.go`. `github.com/spf13/cobra`
is removed from `go.mod`.

All interactive navigation is driven from `internal/tui/tui.go`. Esc is bound
alongside ctrl+c as the abort key through a shared `runForm` helper so every
form treats Esc as "back" without having to repeat the keymap in each call
site.

## Consequences

There is now a single navigation surface: when a menu option is added or
removed in `tui.go`, the change is immediately reflected in the running
binary. Dead-code and "but the subcommand still works" surprises go away.

The cost is that shell-level shortcuts are gone. Users who muscle-memoried
`af project apply` must now go through the menu. Tab completion for
subcommands no longer applies. If a future ADR reintroduces non-interactive
invocation (for scripting or CI), it would be a fresh design rather than a
reinstatement of the Cobra tree, because the TUI flows still expect a
terminal.

The `internal/appcmd` package is deleted. `cmd/af/main.go` now contains the
whole entry point: parse `--registry`, build an `app.Service`, call
`tui.Run`.
