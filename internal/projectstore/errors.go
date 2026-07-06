package projectstore

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// OrphanProfileIDError reports a projects.json group whose profile id has
// no matching profile in the registry. projects.json is owned by the app
// (not user-edited), so an orphan is data corruption. Load surfaces it
// instead of silently pruning the group.
type OrphanProfileIDError struct {
	ProfileID string
}

func (e OrphanProfileIDError) Error() string {
	return fmt.Sprintf("orphan projects group: profile %s is not registered", e.ProfileID)
}

func (OrphanProfileIDError) Severity() errs.Severity {
	return errs.SeverityError
}

// DuplicateProjectIDError reports an attempt to Add a project whose id
// already exists inside the same profile group.
type DuplicateProjectIDError struct {
	ProfileID string
	ProjectID string
}

func (e DuplicateProjectIDError) Error() string {
	return fmt.Sprintf("duplicate project id %q under profile %s", e.ProjectID, e.ProfileID)
}

func (DuplicateProjectIDError) Severity() errs.Severity {
	return errs.SeverityError
}

// ProjectNotFoundError reports an Update or Remove call for a project
// that does not exist in the given profile group.
type ProjectNotFoundError struct {
	ProfileID string
	ProjectID string
}

func (e ProjectNotFoundError) Error() string {
	return fmt.Sprintf("project not found: profile %s, project %s", e.ProfileID, e.ProjectID)
}

func (ProjectNotFoundError) Severity() errs.Severity {
	return errs.SeverityError
}
