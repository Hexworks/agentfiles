package app

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/surfaces"
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

// FolderNotRegisterableError reports a CreateAssetFromFolder (or
// Apply → ignored_paths) call whose dirKey does not pass the
// registration rule in the freshly computed plan. Reason distinguishes
// the three rejection modes so the TUI can render a corrective
// message specific to the failure:
//
//   - surfaces.ReasonNotUnderContainerRoot — dirKey's parent is not a
//     known asset-container root (see surfaces.AssetContainerRoots).
//     Either dirKey sits above a container root, is nested deeper than
//     a direct child of one, or lives outside every asset root.
//   - surfaces.ReasonHasManagedDescendants — dirKey is a valid direct
//     child of a container root but at least one leaf under it is
//     already managed; registering the folder would clobber managed
//     files.
//   - surfaces.ReasonAbsentFromPlan — dirKey is a valid direct child of
//     a container root but no leaf under it appears in the plan (the
//     folder is empty, was deleted between the TUI listing and apply,
//     or the caller invented a stale key).
//
// The service re-asserts eligibility rather than trusting the TUI
// button gate, so a stale or tampered key cannot register a
// partly-managed folder.
type FolderNotRegisterableError struct {
	DirKey string
	Reason surfaces.FolderRejectionReason
}

func (e FolderNotRegisterableError) Error() string {
	switch e.Reason {
	case surfaces.ReasonNotUnderContainerRoot:
		return fmt.Sprintf("folder is not directly under an asset-container root: %s", e.DirKey)
	case surfaces.ReasonHasManagedDescendants:
		return fmt.Sprintf("folder contains files already managed by agentfiles: %s", e.DirKey)
	case surfaces.ReasonAbsentFromPlan:
		return fmt.Sprintf("folder is not present in the current plan: %s", e.DirKey)
	default:
		return fmt.Sprintf("folder is not registerable as an asset: %s", e.DirKey)
	}
}

func (FolderNotRegisterableError) Severity() errs.Severity {
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

// ProfileFolderRemoveError reports a non-recoverable failure while
// removing the on-disk profile folder during DeleteProfile. A pre-missing
// folder is not an error: only real filesystem failures (permissions,
// I/O) reach the caller through this value.
type ProfileFolderRemoveError struct {
	Path string
	Err  error
}

func (e ProfileFolderRemoveError) Error() string {
	return fmt.Sprintf("remove profile folder %s: %s", e.Path, e.Err.Error())
}

func (ProfileFolderRemoveError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e ProfileFolderRemoveError) Unwrap() error {
	return e.Err
}

// ProfileFolderNotARootError reports that DeleteProfileWithFolder
// refused to recurse a path that no longer looks like a profile root
// (its profile.json is missing). Defends against a tampered or stale
// registry entry pointing at an arbitrary directory.
type ProfileFolderNotARootError struct {
	Path string
}

func (e ProfileFolderNotARootError) Error() string {
	return fmt.Sprintf("path is not a profile root (missing profile.json): %s", e.Path)
}

func (ProfileFolderNotARootError) Severity() errs.Severity {
	return errs.SeverityError
}

// UnsafeProfilePathError reports that DeleteProfileWithFolder refused a
// pathological deletion target: the empty string, the filesystem root,
// the user's home directory, or an ancestor of the profile registry
// file. Reason describes which rule was matched so the TUI can render a
// specific message.
type UnsafeProfilePathError struct {
	Path   string
	Reason string
}

func (e UnsafeProfilePathError) Error() string {
	return fmt.Sprintf("refusing to delete unsafe profile path (%s): %s", e.Reason, e.Path)
}

func (UnsafeProfilePathError) Severity() errs.Severity {
	return errs.SeverityError
}
