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
