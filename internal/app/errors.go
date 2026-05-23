package app

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// AssetNotFoundError reports an asset id passed to AddProject that is not
// present in the selected profile. The render package emits a value of
// the same name when a stored project manifest references a missing
// asset; both report the same situation, qualified by package.
type AssetNotFoundError struct {
	AssetID string
}

func (e AssetNotFoundError) Error() string {
	return fmt.Sprintf("unknown asset: %s", e.AssetID)
}

func (AssetNotFoundError) Severity() errs.Severity {
	return errs.SeverityError
}

// AssetExistsError reports an attempt to scaffold an asset whose id already
// exists in the target profile.
type AssetExistsError struct {
	AssetID string
}

func (e AssetExistsError) Error() string {
	return fmt.Sprintf("asset already exists: %s", e.AssetID)
}

func (AssetExistsError) Severity() errs.Severity {
	return errs.SeverityError
}

// ProjectNotFoundError reports a project id that does not exist inside the
// loaded profile.
type ProjectNotFoundError struct {
	ProjectID string
}

func (e ProjectNotFoundError) Error() string {
	return fmt.Sprintf("project not found: %s", e.ProjectID)
}

func (ProjectNotFoundError) Severity() errs.Severity {
	return errs.SeverityError
}

// ProjectPathOwnedError reports an attempt to register a project path that
// already belongs to a project. The owning project may live in the active
// profile or in another registered profile; both are reported so the caller
// can resolve the conflict precisely.
type ProjectPathOwnedError struct {
	Path        string
	ProfileName string
	ProjectName string
}

func (e ProjectPathOwnedError) Error() string {
	return fmt.Sprintf("project path %s already owned by project %q in profile %s", e.Path, e.ProjectName, e.ProfileName)
}

func (ProjectPathOwnedError) Severity() errs.Severity {
	return errs.SeverityWarning
}
