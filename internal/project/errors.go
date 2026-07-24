package project

import (
	"fmt"
	"strings"

	"github.com/hexworks/agentfiles/internal/agent"
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

// UnknownEnabledAgentError reports a project manifest whose enabled_agents
// slice names one or more agent ids not in the recognized set
// (agent.All). Every unknown id found is carried in Agents so the whole
// batch is reported in one pass.
type UnknownEnabledAgentError struct {
	Agents []agent.Agent
}

func (e UnknownEnabledAgentError) Error() string {
	return fmt.Sprintf("unknown enabled agent(s): %s", strings.Join(agent.Strings(e.Agents), ", "))
}

func (UnknownEnabledAgentError) Severity() errs.Severity {
	return errs.SeverityError
}
