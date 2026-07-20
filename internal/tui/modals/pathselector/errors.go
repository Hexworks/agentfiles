package pathselector

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// StartOutsideConstraintError is returned by [New] when the caller-supplied
// StartFolder resolves outside the Constraint root.
type StartOutsideConstraintError struct {
	Start      string
	Constraint string
}

func (e StartOutsideConstraintError) Error() string {
	return fmt.Sprintf("start folder %q is outside constraint %q", e.Start, e.Constraint)
}

func (StartOutsideConstraintError) Severity() errs.Severity { return errs.SeverityError }

// StartUnreadableError is returned by [New] when the caller-supplied
// StartFolder does not exist, is not a directory, or cannot be read.
type StartUnreadableError struct {
	Path string
	Err  error
}

func (e StartUnreadableError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("start folder %q is not readable", e.Path)
	}
	return fmt.Sprintf("start folder %q is not readable: %s", e.Path, e.Err.Error())
}

func (StartUnreadableError) Severity() errs.Severity { return errs.SeverityError }

func (e StartUnreadableError) Unwrap() error { return e.Err }

// ConstraintUnreadableError is returned by [New] when Constraint is set but
// does not exist or is not a directory.
type ConstraintUnreadableError struct {
	Path string
	Err  error
}

func (e ConstraintUnreadableError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("constraint %q is not readable", e.Path)
	}
	return fmt.Sprintf("constraint %q is not readable: %s", e.Path, e.Err.Error())
}

func (ConstraintUnreadableError) Severity() errs.Severity { return errs.SeverityError }

func (e ConstraintUnreadableError) Unwrap() error { return e.Err }

// ReadDirError is emitted (as a notification) when a runtime os.ReadDir on the
// browsed folder fails — permission denied, folder disappeared, etc. The modal
// does not resolve on this class of error; it stays on the previous folder and
// surfaces the notification.
type ReadDirError struct {
	Path string
	Err  error
}

func (e ReadDirError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("cannot read directory %q", e.Path)
	}
	return fmt.Sprintf("cannot read directory %q: %s", e.Path, e.Err.Error())
}

func (ReadDirError) Severity() errs.Severity { return errs.SeverityError }

func (e ReadDirError) Unwrap() error { return e.Err }
