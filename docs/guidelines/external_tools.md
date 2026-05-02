# External Tools Guidelines

Related: [Security](security.md), [TUI](tui.md), [Clean Architecture](clean_architecture.md).

`agentfiles` occasionally needs to hand control to a tool that lives outside the
process — most commonly the user's text editor. This guideline describes how
those handoffs should be implemented so they stay safe, predictable, and easy to
test, and so the TUI keeps owning the user experience around them.

The reference implementation is `internal/fsutil/editor`, which opens the system
editor from a Bubble Tea program. The decision to extract it into its own
package is recorded in ADR [0009](../adr/0009-edit-files-via-system-editor.md).

## Core Rule

Wrap each external tool in its own narrow package. The package owns process
construction, environment lookup, argument quoting, and the message it sends
back when the tool finishes. Callers should not reach for `os/exec`,
`os.Getenv`, or shell parsing themselves.

```text
Do:
- expose one function per tool that returns either a tea.Cmd (when the tool
  must take over the terminal) or a synchronous result (when it cannot)
- give the package a single FinishedMsg-style type so the TUI can pattern-match
- keep $VISUAL / $EDITOR / fallback rules and shell-quoting inside the package
```

```text
Don't:
- call exec.Command from a TUI screen
- inspect os.Getenv("EDITOR") from a TUI screen
- build shell command lines with fmt.Sprintf and string concatenation in
  domain or app code
```

## Suspending The TUI

When the external tool needs the terminal — editors, pagers, interactive merge
tools — use `tea.ExecProcess`. The Bubble Tea runtime releases and restores the
terminal automatically, so the wrapper just produces the command and the
finished message.

```go
// internal/fsutil/editor/editor.go
func Open(path string) tea.Cmd {
    if abs, err := filepath.Abs(path); err == nil {
        path = abs
    }
    line := fmt.Sprintf("%s %s", Resolve(), shellQuote(path))
    cmd := exec.Command("sh", "-c", line)
    return tea.ExecProcess(cmd, func(err error) tea.Msg {
        return FinishedMsg{Err: err}
    })
}
```

The caller's `Update` loop reacts to `FinishedMsg` and, if needed, refreshes
state from disk:

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    if _, ok := msg.(editor.FinishedMsg); ok {
        m.files = listFiles()
        return m, nil
    }
    // ...
}
```

Do not set `cmd.Stdin`, `cmd.Stdout`, or `cmd.Stderr` yourself. Bubble Tea
wires the program's tty into the child process for `ExecProcess`; overriding
those fields can swap the child onto a different file descriptor than the one
that owns the terminal.

## Resolving The Tool

Editor selection should follow standard Unix priority: `$VISUAL` first, then
`$EDITOR`, then a hard fallback. Trim whitespace before deciding "set or not."

```go
func Resolve() string {
    if e := strings.TrimSpace(os.Getenv("VISUAL")); e != "" {
        return e
    }
    if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
        return e
    }
    return FallbackEditor
}
```

Apply the same shape to other tools: prefer the most specific environment
variable the tool documents, fall back to a less specific one, then to a sane
default.

## Quoting

`$EDITOR` is a command line, not a binary name. Users frequently set values
like `nvim --clean`, `code -w`, or even an `alias`-style wrapper. Three
options exist:

1. Run the command through `sh -c` and let the user's shell parse it.
2. Split on whitespace with `strings.Fields` and call `exec.Command`
   directly.
3. Require the user to set a binary-only path.

Prefer option 1. It defers parsing to the shell the user already configured
and handles flag values with quoted spaces correctly. The cost is one shell
process per invocation, which is negligible compared to the interactive
session that follows.

When option 1 is used, the file path must be quoted defensively:

```go
func shellQuote(s string) string {
    return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
```

Wrapping in single quotes blocks every shell metacharacter, and the
`'\''` idiom escapes embedded single quotes the standard POSIX way. Never
interpolate a user-controlled value into a shell command line without this
treatment — see also [Security](security.md) on unsafe execution paths.

## Resolving Paths Before Handoff

Resolve user-facing paths to an absolute form before invoking the tool. The
working directory may change between the moment the tool is configured and the
moment it actually runs, and an editor that opens the wrong file because of a
relative path produces silent data loss. `filepath.Abs` is enough; surfacing a
resolution error is optional, since the editor will surface the problem too.

## Working With Forms In Bubble Tea Models

The "type a name, then edit the file" flow is the most common shape that uses
this pattern, and it has a known foot-gun that is not specific to external
tools but that bites here in practice.

Bubble Tea models written with **value receivers** (`func (m model) Update(...)`)
copy the model on every `Update` cycle. Binding a `huh` form field with
`Value(&m.field)` captures the address of `field` inside the *current copy*
of the model. After the method returns, that address is dangling — the form
keeps writing to the old stack frame and the model the runtime holds onto
never receives the user's input.

```text
Do:
- tag huh fields with Key("name"), Key("yes"), …
- read values from the form on completion via form.GetString / form.GetBool
- store the form pointer (*huh.Form) in the model so the form's heap state
  survives across Update cycles
```

```text
Don't:
- bind huh fields with Value(&m.someField) when the model uses value receivers
- assume "it worked once" means the binding is safe — the bug is timing-
  sensitive and may produce empty strings only after specific keystrokes
```

The TUI does not need to switch to pointer receivers to use forms safely;
keying the fields and reading them on completion is the smaller change and
keeps the rest of the model code unchanged.

## Errors From External Tools

The wrapper package surfaces failure as `FinishedMsg{Err: error}`. Callers
should treat `Err != nil` as "the user did not finish the edit cleanly":
either re-run, abandon the change, or surface a TUI-level message. Do not
wrap the error into an `errs.DomainError` inside the wrapper package — the
asynchronous message boundary is the wrong place for the domain-error
contract, and the TUI is the only consumer. If a domain operation needs to
fail, that failure should be raised after the TUI re-enters the relevant
flow, not inside the editor wrapper.

## Testing

External-tool wrappers are difficult to test end-to-end because they take
over the terminal. Cover the parts that do not require a real tty:

```text
Do:
- unit-test the editor-resolution function with $VISUAL / $EDITOR / neither set
- unit-test the shell-quoting helper against single quotes, spaces, and
  metacharacters
- exercise the wrapper from a small bubbletea program that uses a stub editor
  (a shell one-liner that writes to the path) when an integration test is
  unavoidable
```

```text
Don't:
- mock os/exec from inside the wrapper to "test" the entire flow — the value
  is in the parts that produce strings, not in the call to the runtime
```
