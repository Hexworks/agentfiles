package help

import "fmt"

// InvalidExtensionError reports that a requested path does not point at a
// markdown file. The help dialog only renders `.md` content from the manual
// root; anything else is rejected before the file system is touched.
type InvalidExtensionError struct {
	Path string
}

func (e *InvalidExtensionError) Error() string {
	return fmt.Sprintf("help: %q is not a markdown (.md) file", e.Path)
}

// OutsideRootError reports that the resolved file would land outside the
// manual root. This catches `..` traversal and absolute paths that escape
// the sandbox.
type OutsideRootError struct {
	Path string
}

func (e *OutsideRootError) Error() string {
	return fmt.Sprintf("help: %q escapes manual root %q", e.Path, ManualRoot)
}

// NotFoundError reports that the resolved file does not exist on disk.
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("help: manual page %q not found", e.Path)
}

// RenderError wraps a glamour rendering failure with the offending path so
// the dialog can surface a useful message instead of the bare library error.
type RenderError struct {
	Path string
	Err  error
}

func (e *RenderError) Error() string {
	return fmt.Sprintf("help: render %q: %s", e.Path, e.Err)
}

func (e *RenderError) Unwrap() error { return e.Err }
