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
