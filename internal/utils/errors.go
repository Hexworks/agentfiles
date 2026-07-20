package utils

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/errs"
)

// PathEmptyError reports a call site that handed an empty path to a
// helper that requires one. Promoted to a typed error so callers cannot
// silently operate on the current working directory.
type PathEmptyError struct{}

func (PathEmptyError) Error() string {
	return "path is empty"
}

func (PathEmptyError) Severity() errs.Severity {
	return errs.SeverityError
}

// ErrPathEmpty is the canonical sentinel value of PathEmptyError.
var ErrPathEmpty = PathEmptyError{}

// EnsureDirError reports a failure to create a directory tree at Path.
// Err preserves the underlying syscall error.
type EnsureDirError struct {
	Path string
	Err  error
}

func (e EnsureDirError) Error() string {
	return fmt.Sprintf("ensure directory %s: %s", e.Path, e.Err.Error())
}

func (EnsureDirError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e EnsureDirError) Unwrap() error {
	return e.Err
}

// ReadJSONError reports a failure to read or decode a JSON file.
type ReadJSONError struct {
	Path string
	Err  error
}

func (e ReadJSONError) Error() string {
	return fmt.Sprintf("read json %s: %s", e.Path, e.Err.Error())
}

func (ReadJSONError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e ReadJSONError) Unwrap() error {
	return e.Err
}

// WriteJSONError reports a failure to encode or write a JSON file.
type WriteJSONError struct {
	Path string
	Err  error
}

func (e WriteJSONError) Error() string {
	return fmt.Sprintf("write json %s: %s", e.Path, e.Err.Error())
}

func (WriteJSONError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e WriteJSONError) Unwrap() error {
	return e.Err
}

// WriteFileError reports a failure to write bytes to Path.
type WriteFileError struct {
	Path string
	Err  error
}

func (e WriteFileError) Error() string {
	return fmt.Sprintf("write file %s: %s", e.Path, e.Err.Error())
}

func (WriteFileError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e WriteFileError) Unwrap() error {
	return e.Err
}

// HashFileError reports a failure to read the file targeted by HashFile.
type HashFileError struct {
	Path string
	Err  error
}

func (e HashFileError) Error() string {
	return fmt.Sprintf("hash file %s: %s", e.Path, e.Err.Error())
}

func (HashFileError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e HashFileError) Unwrap() error {
	return e.Err
}

// CopyDirError reports a failure while recursively copying a directory
// tree from Src to Dst. Err preserves the underlying read/write error.
type CopyDirError struct {
	Src string
	Dst string
	Err error
}

func (e CopyDirError) Error() string {
	return fmt.Sprintf("copy directory %s -> %s: %s", e.Src, e.Dst, e.Err.Error())
}

func (CopyDirError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e CopyDirError) Unwrap() error {
	return e.Err
}

// DirStatsError reports a failure while summarizing a directory tree in
// DirStats. Err preserves the underlying walk error.
type DirStatsError struct {
	Root string
	Err  error
}

func (e DirStatsError) Error() string {
	return fmt.Sprintf("stat directory %s: %s", e.Root, e.Err.Error())
}

func (DirStatsError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e DirStatsError) Unwrap() error {
	return e.Err
}

// AbsPathError reports a failure to resolve a path to an absolute form.
type AbsPathError struct {
	Path string
	Err  error
}

func (e AbsPathError) Error() string {
	return fmt.Sprintf("absolute path %s: %s", e.Path, e.Err.Error())
}

func (AbsPathError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e AbsPathError) Unwrap() error {
	return e.Err
}

// PathResolutionError reports a failure to symlink-resolve Path. Returned
// by ResolveAbs when follow is true and filepath.EvalSymlinks fails — a
// dangling symlink, a permission-denied on a path component, or a race
// that removed the target between Abs and EvalSymlinks all surface here.
// Callers treat this as a containment failure: the path cannot be proven
// to sit under any expected root, so it must not be handed to the
// filesystem.
type PathResolutionError struct {
	Path string
	Err  error
}

func (e PathResolutionError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("resolve path %s", e.Path)
	}
	return fmt.Sprintf("resolve path %s: %s", e.Path, e.Err.Error())
}

func (PathResolutionError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e PathResolutionError) Unwrap() error {
	return e.Err
}
