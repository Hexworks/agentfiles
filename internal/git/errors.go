package git

import (
	"fmt"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// BinaryMissingError reports that the `git` binary is not on PATH. Both
// the settings pre-flight and the commit site can produce this — the
// pre-flight refuses the setting save; the commit path treats it as a
// warn-and-skip.
type BinaryMissingError struct {
	Err error
}

func (BinaryMissingError) Error() string {
	return "git binary not found on PATH"
}

func (BinaryMissingError) Severity() errs.Severity {
	return errs.SeverityWarning
}

func (e BinaryMissingError) Unwrap() error {
	return e.Err
}

// NotARepoError reports that a directory is not inside a git work tree.
// Detect returns it so callers can treat "not a git repo" as a silent
// skip rather than a hard failure.
type NotARepoError struct {
	Dir string
}

func (e NotARepoError) Error() string {
	return fmt.Sprintf("%s is not a git work tree", e.Dir)
}

func (NotARepoError) Severity() errs.Severity {
	return errs.SeverityInfo
}

// UnrelatedStagedChangesError reports staged paths outside the caller's
// pathspec. Commit refuses so unrelated user work is never rolled into
// an automated commit.
type UnrelatedStagedChangesError struct {
	Paths []string
}

func (e UnrelatedStagedChangesError) Error() string {
	return "unrelated staged changes present: " + strings.Join(e.Paths, ", ")
}

func (UnrelatedStagedChangesError) Severity() errs.Severity {
	return errs.SeverityWarning
}

// HookFailedError reports a non-zero commit exit whose stderr begins
// with a hook diagnostic. Surfaced separately from CommitError so the
// TUI can show the hook message plainly.
type HookFailedError struct {
	Stderr string
}

func (e HookFailedError) Error() string {
	return "git hook rejected commit: " + firstLine(e.Stderr)
}

func (HookFailedError) Severity() errs.Severity {
	return errs.SeverityWarning
}

// CommitError reports a generic commit failure. Stderr carries the raw
// git output so the TUI can render it.
type CommitError struct {
	Stderr string
}

func (e CommitError) Error() string {
	return "git commit failed: " + firstLine(e.Stderr)
}

func (CommitError) Severity() errs.Severity {
	return errs.SeverityWarning
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
