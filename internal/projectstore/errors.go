package projectstore

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// HomeDirUnavailableError reports that os.UserHomeDir failed or returned
// empty when computing DefaultPath. Surfaced instead of silently
// returning a CWD-relative fallback so the environment problem is
// visible to the caller.
type HomeDirUnavailableError struct {
	Err error
}

func (HomeDirUnavailableError) Error() string {
	return "user home directory unavailable"
}

func (HomeDirUnavailableError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e HomeDirUnavailableError) Unwrap() error {
	return e.Err
}

// ProjectPathOwnedError reports an Add or Update whose target repo
// path is already claimed by another project across the registered
// profiles. The invariant lives inside the projects store so any
// caller — the TUI, a future scripting path, a migration — sees the
// same rule; app.Service translates the profile id into a human name
// for the TUI.
type ProjectPathOwnedError struct {
	Path              string
	ExistingProfileID string
	ExistingProjectID string
}

func (e ProjectPathOwnedError) Error() string {
	return fmt.Sprintf("project path already owned: %s (profile %s, project %s)", e.Path, e.ExistingProfileID, e.ExistingProjectID)
}

func (ProjectPathOwnedError) Severity() errs.Severity {
	return errs.SeverityWarning
}

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
