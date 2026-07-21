package sync

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// StateMissingError reports that no managed-state snapshot exists at
// the expected path inside the project repository. This is benign for
// first-time applies; loadState reports it so callers can distinguish
// "never applied" from "state file unreadable".
type StateMissingError struct {
	Path string
}

func (e StateMissingError) Error() string {
	return fmt.Sprintf("managed state missing: %s", e.Path)
}

func (StateMissingError) Severity() errs.Severity {
	return errs.SeverityInfo
}

// StateCorruptError reports that the managed-state file decoded but
// contained a managed-file key that is either absolute or escapes the
// project root via "..". Such keys cannot be trusted to scope writes
// or deletes, so the entire state file is rejected.
type StateCorruptError struct {
	Path string
	Key  string
}

func (e StateCorruptError) Error() string {
	return fmt.Sprintf("managed state %s: unsafe key %q", e.Path, e.Key)
}

func (StateCorruptError) Severity() errs.Severity {
	return errs.SeverityError
}

// DeleteError reports a failure to remove a file during Apply. Covers
// both ChangeDelete entries (state-recorded files no longer in desired)
// and ChangeUnknown entries resolved with UnknownDelete.
type DeleteError struct {
	Path string
	Err  error
}

func (e DeleteError) Error() string {
	return fmt.Sprintf("delete file %s: %s", e.Path, e.Err.Error())
}

func (DeleteError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e DeleteError) Unwrap() error {
	return e.Err
}

// StatError reports a failure to stat a managed-surface root while
// detecting extraneous files (the deletes/unknowns split inside
// detectDeletesAndUnknowns).
type StatError struct {
	Path string
	Err  error
}

func (e StatError) Error() string {
	return fmt.Sprintf("stat %s: %s", e.Path, e.Err.Error())
}

func (StatError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e StatError) Unwrap() error {
	return e.Err
}

// SurfaceWalkError reports a failure encountered while walking one of
// the managed-surface roots during extraneous-file detection.
type SurfaceWalkError struct {
	Root string
	Err  error
}

func (e SurfaceWalkError) Error() string {
	return fmt.Sprintf("walk surface %s: %s", e.Root, e.Err.Error())
}

func (SurfaceWalkError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e SurfaceWalkError) Unwrap() error {
	return e.Err
}

// SurfaceSymlinkError reports that a managed-surface root resolved to
// a symbolic link. The walk refuses to descend through symlinks so
// stray files in unrelated directories cannot be classified as
// ChangeUnknown and later deleted via a UnknownDelete resolution.
type SurfaceSymlinkError struct {
	Root string
}

func (e SurfaceSymlinkError) Error() string {
	return fmt.Sprintf("managed surface root is a symlink: %s", e.Root)
}

func (SurfaceSymlinkError) Severity() errs.Severity {
	return errs.SeverityWarning
}

// UnsafeSymlinkError reports that Apply refused to write through a
// symbolic link at the target path. Writing would have followed the
// link to an unrelated file outside the managed-state hash invariant.
type UnsafeSymlinkError struct {
	Path string
}

func (e UnsafeSymlinkError) Error() string {
	return fmt.Sprintf("refusing to write through symlink: %s", e.Path)
}

func (UnsafeSymlinkError) Severity() errs.Severity {
	return errs.SeverityError
}

// InvalidPathError reports a caller-supplied resolution or a target
// path that does not fit the slash-key convention used by FileChange.
// Absolute paths and OS-separated paths are rejected; only forward-
// slash relative keys reach the apply loop.
type InvalidPathError struct {
	Path   string
	Reason string
}

func (e InvalidPathError) Error() string {
	return fmt.Sprintf("invalid path %q: %s", e.Path, e.Reason)
}

func (InvalidPathError) Severity() errs.Severity {
	return errs.SeverityError
}

// OutsideSurfaceError reports an Apply target that does not fall
// inside a managed-surface root. Combined with InvalidPathError this
// gives defense-in-depth for any path that flowed through Plan into a
// FileChange entry.
type OutsideSurfaceError struct {
	Path string
}

func (e OutsideSurfaceError) Error() string {
	return fmt.Sprintf("path is outside managed surfaces: %s", e.Path)
}

func (OutsideSurfaceError) Severity() errs.Severity {
	return errs.SeverityError
}

// PreviewInvariantError reports that Apply encountered a preview whose
// shape violates an invariant proven by Plan (e.g. a ChangeDrift row
// paired with a nil ManagedState). Emitting a typed error surfaces the
// invariant break loudly instead of silently dropping state.
type PreviewInvariantError struct {
	Kind   string
	Reason string
}

func (e PreviewInvariantError) Error() string {
	return fmt.Sprintf("preview invariant violated (%s): %s", e.Kind, e.Reason)
}

func (PreviewInvariantError) Severity() errs.Severity {
	return errs.SeverityError
}
