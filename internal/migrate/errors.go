package migrate

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// ReadV1RegistryError reports a failure to read or decode the legacy
// registry file. Fatal because Run cannot faithfully preserve profiles it
// cannot read.
type ReadV1RegistryError struct {
	Path string
	Err  error
}

func (e ReadV1RegistryError) Error() string {
	return fmt.Sprintf("read v1 registry %s: %s", e.Path, e.Err.Error())
}

func (ReadV1RegistryError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e ReadV1RegistryError) Unwrap() error {
	return e.Err
}

// HarvestProjectsError reports a failure to enumerate a profile's legacy
// projects/ directory. Fatal because silently skipping the directory
// would drop user selections without warning.
type HarvestProjectsError struct {
	Path string
	Err  error
}

func (e HarvestProjectsError) Error() string {
	return fmt.Sprintf("harvest v1 projects %s: %s", e.Path, e.Err.Error())
}

func (HarvestProjectsError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e HarvestProjectsError) Unwrap() error {
	return e.Err
}
