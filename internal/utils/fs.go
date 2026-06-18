package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// CopyDir recursively copies every regular file under src into dst,
// preserving the relative directory structure. Copied files are written
// with a fixed 0o644 mode — the source folder is untrusted, so its
// permission bits are never reproduced inside the profile. Parent
// directories under dst are created as needed. Symlinks and other
// non-regular entries are skipped so the copy never follows a link out of
// the source tree.
func CopyDir(src, dst string) errs.DomainError {
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel := ToRelative(src, path)
		if writeErr := WriteFile(filepath.Join(dst, filepath.FromSlash(rel)), data, 0o644); writeErr != nil {
			return writeErr
		}
		return nil
	})
	if walkErr == nil {
		return nil
	}
	// WriteFile already returns a typed DomainError; surface it directly
	// instead of nesting it inside CopyDirError. Only raw os failures
	// (ReadFile, the walk error itself) get the copy-dir wrap.
	var domainErr errs.DomainError
	if errors.As(walkErr, &domainErr) {
		return domainErr
	}
	return CopyDirError{Src: src, Dst: dst, Err: walkErr}
}

// DirStats returns the count and total byte size of the regular files under
// root, matching CopyDir's traversal (non-regular entries skipped). It is a
// read-only summary used to show what a folder-register copy will move
// before the user confirms.
func DirStats(root string) (count int, size int64, err errs.DomainError) {
	walkErr := filepath.WalkDir(root, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		count++
		size += info.Size()
		return nil
	})
	if walkErr != nil {
		return 0, 0, DirStatsError{Root: root, Err: walkErr}
	}
	return count, size, nil
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
