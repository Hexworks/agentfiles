package asset

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// AssetIDNameRequiredError reports a manifest whose id or name field is
// empty. The exported zero-value sentinel ErrAssetIDNameRequired matches
// the standard-library "io.EOF"-style convention; callers can also use
// errors.As(err, new(AssetIDNameRequiredError)) for symmetry with the
// other typed errors in this package.
type AssetIDNameRequiredError struct{}

func (AssetIDNameRequiredError) Error() string {
	return "asset id and name are required"
}

func (AssetIDNameRequiredError) Severity() errs.Severity {
	return errs.SeverityError
}

// ErrAssetIDNameRequired is the canonical sentinel value of
// AssetIDNameRequiredError. Callers may compare with errors.Is or
// introspect with errors.As; both work because the type carries no
// fields.
var ErrAssetIDNameRequired = AssetIDNameRequiredError{}

// MissingContentFileError reports a folder-register attempt whose source
// folder lacks the file the chosen convention-based type requires (e.g. a
// skill without SKILL.md). Without it the copied asset would render nothing,
// so the registration is rejected up front.
type MissingContentFileError struct {
	Type Type
	File string
}

func (e MissingContentFileError) Error() string {
	return fmt.Sprintf("a %s asset requires %s in the source folder", e.Type, e.File)
}

func (MissingContentFileError) Severity() errs.Severity {
	return errs.SeverityError
}

// MissingProjectionsError reports a folder-register attempt for a generic
// type (mcp, rule, hook) whose manifest carries no projections. Generic types
// render only via explicit projections, so without them the copied content
// would never reach a managed surface.
type MissingProjectionsError struct {
	Type Type
}

func (e MissingProjectionsError) Error() string {
	return fmt.Sprintf("a %s asset requires at least one projection", e.Type)
}

func (MissingProjectionsError) Severity() errs.Severity {
	return errs.SeverityError
}

// ProjectionOutsideSurfacesError reports a projection whose target falls
// outside the managed-surface fence. Render would reject such a plan, so the
// folder-register flow rejects it before copying any content.
type ProjectionOutsideSurfacesError struct {
	Target string
}

func (e ProjectionOutsideSurfacesError) Error() string {
	return fmt.Sprintf("projection target outside managed surfaces: %s", e.Target)
}

func (ProjectionOutsideSurfacesError) Severity() errs.Severity {
	return errs.SeverityError
}

// UnsupportedAssetTypeError reports an asset manifest whose Type field is not
// one of the supported asset.Type constants. The original Type value is
// preserved so callers can render a precise message.
type UnsupportedAssetTypeError struct {
	Type Type
}

func (e UnsupportedAssetTypeError) Error() string {
	return fmt.Sprintf("unsupported asset type: %s", e.Type)
}

func (UnsupportedAssetTypeError) Severity() errs.Severity {
	return errs.SeverityError
}

// AssetWalkError reports a failure encountered while walking an asset
// directory in RelativeFiles. RelPath is asset-relative so absolute
// paths inside the user's profile root never reach the user.
type AssetWalkError struct {
	AssetDir string
	Err      error
}

func (e AssetWalkError) Error() string {
	return fmt.Sprintf("walk asset directory %s: %s", e.AssetDir, e.Err.Error())
}

func (AssetWalkError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e AssetWalkError) Unwrap() error {
	return e.Err
}

// AssetFolderRemoveError reports a non-recoverable failure while removing
// an asset directory during Delete. A pre-missing directory is not an
// error.
type AssetFolderRemoveError struct {
	Dir string
	Err error
}

func (e AssetFolderRemoveError) Error() string {
	return fmt.Sprintf("remove asset folder %s: %s", e.Dir, e.Err.Error())
}

func (AssetFolderRemoveError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e AssetFolderRemoveError) Unwrap() error {
	return e.Err
}

// FileRemoveError reports a failure while deleting a file inside an asset
// directory.
type FileRemoveError struct {
	Path string
	Err  error
}

func (e FileRemoveError) Error() string {
	return fmt.Sprintf("remove asset file %q: %v", e.Path, e.Err)
}

func (FileRemoveError) Severity() errs.Severity { return errs.SeverityError }

func (e FileRemoveError) Unwrap() error { return e.Err }

// FileCreateError reports a failure while creating a file inside an asset
// directory (either the mkdir-all of the parent or the write itself).
type FileCreateError struct {
	Path string
	Err  error
}

func (e FileCreateError) Error() string {
	return fmt.Sprintf("create asset file %q: %v", e.Path, e.Err)
}

func (FileCreateError) Severity() errs.Severity { return errs.SeverityError }

func (e FileCreateError) Unwrap() error { return e.Err }

// FilePathError reports a relative path that resolves outside the asset
// directory or names a reserved file (the asset manifest itself, or a
// hidden dotfile that RelativeFiles would refuse to list anyway).
type FilePathError struct {
	Path   string
	Reason string
}

func (e FilePathError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("asset file path %q is rejected", e.Path)
	}
	return fmt.Sprintf("asset file path %q is rejected: %s", e.Path, e.Reason)
}

func (FilePathError) Severity() errs.Severity { return errs.SeverityError }
