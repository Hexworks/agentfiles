package doctor

import (
	"fmt"

	"github.com/addamsson/agentfiles/internal/errs"
)

// ProjectCheckError reports a per-project plan failure encountered while
// building the report. Err preserves the underlying cause so callers can
// inspect domain leaves with errs.Collect.
type ProjectCheckError struct {
	ProjectName string
	Err         error
}

func (e ProjectCheckError) Error() string {
	return fmt.Sprintf("project %s: %s", e.ProjectName, e.Err.Error())
}

func (ProjectCheckError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e ProjectCheckError) Unwrap() error {
	return e.Err
}
