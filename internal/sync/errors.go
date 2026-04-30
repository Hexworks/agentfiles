package sync

import (
	"fmt"

	"github.com/addamsson/agentfiles/internal/errs"
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

// DeleteError reports a failure to remove a delete-candidate file
// during Apply.
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
// detecting delete candidates.
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
// the managed-surface roots for delete-candidate detection.
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
