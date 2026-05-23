// Package fsutil gathers the small filesystem and hashing helpers shared by the
// rest of the codebase. It keeps path handling, JSON I/O, and content hashing
// consistent so higher layers do not have to repeat the same boilerplate.
//
// Every fallible helper returns an errs.DomainError so the rest of the
// codebase can keep its "errors are domain values" contract end-to-end.
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// ExpandHome resolves a leading "~" or "~/" in path against the current user's
// home directory. Paths without a tilde prefix are returned unchanged.
func ExpandHome(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// EnsureDir creates the directory at path (and any missing parents) with 0755
// permissions, returning nil if it already exists. 0755 means the owner has
// read, write, and execute permission (7), while group and others have read
// and execute but not write (5) — the standard mode for user-owned directories
// that should be traversable by everyone but only modifiable by the owner.
func EnsureDir(path string) errs.DomainError {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return EnsureDirError{Path: path, Err: err}
	}
	return nil
}

// Exists reports whether anything exists at path. Any stat error is treated as
// "does not exist", which is good enough for the existence checks in this
// project.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadJSON reads the file at path and decodes its contents into v.
// TODO: make this a generic function (@see task#0001)
func ReadJSON(path string, v any) errs.DomainError {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReadJSONError{Path: path, Err: err}
	}
	if err := json.Unmarshal(data, v); err != nil {
		return ReadJSONError{Path: path, Err: err}
	}
	return nil
}

// WriteJSON marshals v as pretty-printed JSON with a trailing newline and
// writes it to path, creating parent directories as needed.
// TODO: make this a generic function (@see task#0001)
func WriteJSON(path string, v any) errs.DomainError {
	if dirErr := EnsureDir(filepath.Dir(path)); dirErr != nil {
		return dirErr
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return WriteJSONError{Path: path, Err: err}
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return WriteJSONError{Path: path, Err: err}
	}
	return nil
}

// WriteFile writes data to path with the given mode, creating parent
// directories as needed.
func WriteFile(path string, data []byte, mode fs.FileMode) errs.DomainError {
	if dirErr := EnsureDir(filepath.Dir(path)); dirErr != nil {
		return dirErr
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return WriteFileError{Path: path, Err: err}
	}
	return nil
}

// HashBytes returns the hex-encoded SHA-256 digest of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashFile returns the hex-encoded SHA-256 digest of the file at path.
func HashFile(path string) (string, errs.DomainError) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", HashFileError{Path: path, Err: err}
	}
	return HashBytes(data), nil
}

// ToRelative returns target as a forward-slash relative path against base. If the
// relative path cannot be computed, target is returned unchanged so callers
// always get a usable string.
func ToRelative(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return filepath.ToSlash(rel)
}

// ToAbsolute expands "~" and resolves path to an absolute form. It returns an
// error for the empty string so callers cannot silently operate on the current
// working directory.
func ToAbsolute(path string) (string, errs.DomainError) {
	path = ExpandHome(path)
	if path == "" {
		return "", ErrPathEmpty
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", AbsPathError{Path: path, Err: err}
	}
	return abs, nil
}
