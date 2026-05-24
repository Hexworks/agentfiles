// Package editor opens the user's system editor from inside a Bubble Tea
// program and resumes the program when the editor exits.
//
// The pattern is intentionally narrow: a TUI flow asks the user to edit a
// file, this package suspends the program with tea.ExecProcess, runs the
// editor against the given path, and dispatches a FinishedMsg back to the
// caller's Update loop when the editor returns. The Bubble Tea runtime
// handles releasing and restoring the terminal, so the caller just has to
// pattern-match on FinishedMsg to refresh state.
//
// Editor selection follows the standard Unix convention: $VISUAL is checked
// first, then $EDITOR, with "vi" as the final fallback. The editor command
// is executed through "sh -c" so values that include flags or a wrapper
// (for example "nvim --clean", "code -w") parse the way the user expects in
// their shell. The file path is resolved to an absolute form and quoted for
// the shell to keep the lookup unambiguous and to keep funny filenames
// (spaces, single quotes) from breaking the command line.
package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// FallbackEditor is used when neither $VISUAL nor $EDITOR is set.
const FallbackEditor = "vi"

// FinishedMsg is dispatched when the editor process has exited and the
// Bubble Tea program has regained control of the terminal. Err is non-nil
// only if the editor itself failed to start or exited with an error; a
// successful edit (including the user choosing not to save) produces
// FinishedMsg{Err: nil}.
type FinishedMsg struct {
	Err error
}

// Open returns a tea.Cmd that suspends the calling Bubble Tea program,
// runs the system editor against path, and dispatches a FinishedMsg when
// the editor exits.
//
// The caller is responsible for ensuring the file at path exists or for
// accepting the editor's behavior on a missing path (most editors will
// open an empty buffer associated with the given filename so the first
// save creates the file).
//
// path is resolved with filepath.Abs before invocation. If resolution
// fails the original path is used; the editor will surface the error.
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

// Resolve returns the editor command to run, in $VISUAL → $EDITOR →
// FallbackEditor priority order. The returned value may contain flags
// (e.g. "nvim --clean") and is meant to be handed to a shell, not to
// exec directly.
func Resolve() string {
	if e := strings.TrimSpace(os.Getenv("VISUAL")); e != "" {
		return e
	}
	if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
		return e
	}
	return FallbackEditor
}

// shellQuote wraps s in single quotes and escapes any embedded single
// quotes using the standard '\” idiom, producing a token that POSIX sh
// will treat as a single literal argument.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
