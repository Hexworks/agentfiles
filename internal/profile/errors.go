package profile

import (
	"fmt"

	"github.com/addamsson/agentfiles/internal/errs"
)

// DuplicateAssetIDError reports an asset id that appears in more than one
// asset directory inside a profile. It carries the offending id so the TUI
// can highlight the conflict without reparsing strings.
type DuplicateAssetIDError struct {
	ID string
}

func (e DuplicateAssetIDError) Error() string {
	return fmt.Sprintf("duplicate asset id: %s", e.ID)
}

func (DuplicateAssetIDError) Severity() errs.Severity {
	return errs.SeverityError
}

// DuplicateProjectIDError reports a project id collision inside a profile's
// projects/ directory.
type DuplicateProjectIDError struct {
	ID string
}

func (e DuplicateProjectIDError) Error() string {
	return fmt.Sprintf("duplicate project id: %s", e.ID)
}

func (DuplicateProjectIDError) Severity() errs.Severity {
	return errs.SeverityError
}

// AssetsScanError reports a failure while walking the profile's assets/
// tree (filesystem error from WalkDir, not a per-asset domain failure).
type AssetsScanError struct {
	Root string
	Err  error
}

func (e AssetsScanError) Error() string {
	return fmt.Sprintf("scan assets %s: %s", e.Root, e.Err.Error())
}

func (AssetsScanError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e AssetsScanError) Unwrap() error {
	return e.Err
}

// ProjectsReadDirError reports a failure while listing the profile's
// projects/ directory.
type ProjectsReadDirError struct {
	Root string
	Err  error
}

func (e ProjectsReadDirError) Error() string {
	return fmt.Sprintf("read projects directory %s: %s", e.Root, e.Err.Error())
}

func (ProjectsReadDirError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e ProjectsReadDirError) Unwrap() error {
	return e.Err
}
