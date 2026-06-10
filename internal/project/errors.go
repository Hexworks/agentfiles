package project

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// ProjectFieldsRequiredError reports a project manifest with one or more
// missing required identity fields (id, name, path).
type ProjectFieldsRequiredError struct{}

func (ProjectFieldsRequiredError) Error() string {
	return "project id, name, and path are required"
}

func (ProjectFieldsRequiredError) Severity() errs.Severity {
	return errs.SeverityError
}

// ErrProjectFieldsRequired is the canonical sentinel value of
// ProjectFieldsRequiredError.
var ErrProjectFieldsRequired = ProjectFieldsRequiredError{}

// NoEnabledAgentsError reports a project manifest whose enabled_agents
// slice is empty. At least one agent must be enabled or render has
// nothing to produce.
type NoEnabledAgentsError struct{}

func (NoEnabledAgentsError) Error() string {
	return "at least one agent must be enabled"
}

func (NoEnabledAgentsError) Severity() errs.Severity {
	return errs.SeverityError
}

// ErrNoEnabledAgents is the canonical sentinel value of
// NoEnabledAgentsError.
var ErrNoEnabledAgents = NoEnabledAgentsError{}

// ProjectDeleteError reports a non-recoverable failure to delete a
// project manifest file. A pre-missing file is not an error.
type ProjectDeleteError struct {
	Path string
	Err  error
}

func (e ProjectDeleteError) Error() string {
	return fmt.Sprintf("delete project manifest %s: %s", e.Path, e.Err.Error())
}

func (ProjectDeleteError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e ProjectDeleteError) Unwrap() error {
	return e.Err
}
