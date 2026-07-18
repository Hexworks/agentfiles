package registry

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// ProfileIDExistsError reports an attempt to add a profile whose id is
// already present in the global registry.
type ProfileIDExistsError struct {
	ID string
}

func (e ProfileIDExistsError) Error() string {
	return fmt.Sprintf("profile id already exists: %s", e.ID)
}

func (ProfileIDExistsError) Severity() errs.Severity {
	return errs.SeverityWarning
}

// ProfileNameExistsError reports an attempt to add a profile whose
// display name (case-insensitive) collides with an existing entry.
type ProfileNameExistsError struct {
	Name string
}

func (e ProfileNameExistsError) Error() string {
	return fmt.Sprintf("profile name already exists: %s", e.Name)
}

func (ProfileNameExistsError) Severity() errs.Severity {
	return errs.SeverityWarning
}

// ProfilePathExistsError reports an attempt to add a profile whose
// filesystem path is already registered.
type ProfilePathExistsError struct {
	Path string
}

func (e ProfilePathExistsError) Error() string {
	return fmt.Sprintf("profile path already exists: %s", e.Path)
}

func (ProfilePathExistsError) Severity() errs.Severity {
	return errs.SeverityWarning
}

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

// ProfileNotFoundError reports a Resolve or Touch lookup that did not
// match any registered profile.
type ProfileNotFoundError struct {
	Ref string
}

func (e ProfileNotFoundError) Error() string {
	if e.Ref == "" {
		return "profile not found"
	}
	return fmt.Sprintf("profile not found: %s", e.Ref)
}

func (ProfileNotFoundError) Severity() errs.Severity {
	return errs.SeverityError
}
