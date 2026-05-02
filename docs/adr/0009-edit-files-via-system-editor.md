# Edit Files Via The System Editor

## Status

accepted

## Context

Several existing and planned `agentfiles` flows give the user a chance to
edit a managed file: skill bodies, profile-level rule documents, and other
markdown content that ships through profiles. The TUI cannot host a full
editor of its own — the project intentionally keeps `huh` and `bubbletea`
as the only interaction primitives (ADR
[0005](./0005-tui-as-the-only-user-interface.md),
[0006](./0006-remove-cli-subcommands-in-favor-of-pure-tui.md)) and writing
a tolerable text editor on top of `bubbles/textarea` is a project of its
own.

What the TUI does have is a Bubble Tea program. Bubble Tea exposes
`tea.ExecProcess`, a primitive that suspends the program, releases the
terminal to a child process with the tty inherited, and then restores the
program when the child exits. That is exactly the affordance needed:
hand the file to the user's preferred editor, let the editor own the
terminal for the duration, and pick the program back up afterwards.

A first iteration of this pattern, written inline in `cmd/af/main.go`
during the file-manager spike, exposed two pitfalls that should not be
relearned next time:

1. **`$EDITOR` is a command, not a binary name.** Users routinely set
   values like `nvim --clean` or `code -w`. Calling
   `exec.Command(os.Getenv("EDITOR"), path)` looks for a binary literally
   named `nvim --clean`, which fails; or, with naive whitespace splitting,
   stops working the moment a flag value contains spaces. Running the
   editor command through `sh -c` outsources the parsing to the shell the
   user already configured.
2. **Bubble Tea models with value receivers must not bind `huh` form
   fields via `Value(&m.field)`.** Each `Update` cycle copies the model;
   the address taken inside one cycle is dangling on the next, and the
   form silently writes to the wrong memory. The fix is to tag fields
   with `Key(...)` and read results via `form.GetString` / `form.GetBool`
   on completion. This is unrelated to the editor mechanics but lives in
   the same flow because the "type a filename, then edit" path is exactly
   where the bug bites.

The third recurring concern is editor selection. Unix convention is
`$VISUAL` first (the rich editor), then `$EDITOR` (any editor able to run
on a basic terminal), then a hard fallback. `agentfiles` should follow
that convention so users do not have to redefine an editor preference
that the rest of their environment already respects.

## Decision

Editor invocation is owned by a single, narrow package:
`internal/fsutil/editor`. The package exports two symbols:

- `func Open(path string) tea.Cmd` — returns a Bubble Tea command that
  suspends the program, runs the editor against an absolute version of
  `path`, and dispatches `FinishedMsg` when the editor exits.
- `type FinishedMsg struct{ Err error }` — the message the caller
  pattern-matches on inside its `Update` to refresh state once the
  editor returns control.

The package always invokes the editor through `sh -c`, with the file
path resolved to an absolute form and shell-quoted via the standard
`'\''` idiom. Editor selection follows `$VISUAL` → `$EDITOR` → `vi`.

The package is deliberately placed under `internal/fsutil/editor/`, not
inside `internal/fsutil/fsutil.go`, because:

- The dependency surface is different. `fsutil` is a leaf utility on
  top of `os`, `encoding/json`, and `crypto/sha256`. The editor package
  imports `os/exec` and `github.com/charmbracelet/bubbletea`, neither of
  which the rest of `fsutil` should pull in transitively.
- The contract is different. `fsutil` helpers return `errs.DomainError`
  directly. The editor package returns a `tea.Cmd` whose result lands
  asynchronously as `FinishedMsg`; the caller chooses how to surface
  any failure inside its TUI flow.

The TUI is still the only consumer. `internal/fsutil/editor` does not
import the TUI, but only the TUI (and the demo program currently in
`cmd/af/main.go`) imports it.

## Consequences

The TUI gains a reusable, one-call path for "let the user edit this
file." Future flows that need the affordance — skill editing, rule
edits, profile-level note editing — invoke `editor.Open(path)` and
react to `editor.FinishedMsg`. None of them need to revisit shell
quoting, editor resolution, or `tea.ExecProcess` plumbing.

Editor command parsing is handed to the shell, which means
`$EDITOR="nvim --clean"`, `$EDITOR="code -w"`, and aliases-in-EDITOR all
behave the way the user expects. The cost is one `sh` process per edit,
which is negligible against an interactive editor session.

A new guideline ([External Tools](../guidelines/external_tools.md))
documents the pattern so future contributors do not write a second
copy. The recurring "value-receiver + bound form pointer" bug is
recorded there as well, so the next person to wire a `huh` form into a
Bubble Tea model finds it before the bug finds them.
