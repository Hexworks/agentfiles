package settings

import (
	"github.com/hexworks/agentfiles/internal/errs"
)

// HomeDirUnavailableError reports that os.UserHomeDir failed or returned
// empty when computing DefaultPath. Mirrors projectstore's shape so the
// user-config store family surfaces the same environment problem the
// same way.
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
