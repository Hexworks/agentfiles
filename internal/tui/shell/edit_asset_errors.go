package shell

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// assetFileRemoveError is returned when the screen fails to delete a
// file inside the asset directory. Wraps the underlying os error.
type assetFileRemoveError struct {
	Path string
	Err  error
}

func (e assetFileRemoveError) Error() string {
	return fmt.Sprintf("remove asset file %q: %v", e.Path, e.Err)
}

func (e assetFileRemoveError) Severity() errs.Severity { return errs.SeverityError }

// assetFileCreateError is returned when the screen fails to create a
// file inside the asset directory.
type assetFileCreateError struct {
	Path string
	Err  error
}

func (e assetFileCreateError) Error() string {
	return fmt.Sprintf("create asset file %q: %v", e.Path, e.Err)
}

func (e assetFileCreateError) Severity() errs.Severity { return errs.SeverityError }

// assetFilePathError is returned when a requested relative path resolves
// outside the asset directory after symlink + dot-dot expansion.
type assetFilePathError struct {
	Path string
}

func (e assetFilePathError) Error() string {
	return fmt.Sprintf("asset file path %q escapes the asset folder", e.Path)
}

func (e assetFilePathError) Severity() errs.Severity { return errs.SeverityError }
