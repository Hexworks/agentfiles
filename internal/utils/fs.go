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
	return EnsureDirMode(path, 0o755)
}

// EnsureDirMode is EnsureDir with an explicit mode. User-private state should
// pass 0o700 so no other local user can traverse it — the two centralized
// user-config files (~/.agentfiles/profiles.json and projects.json) hold
// machine-identifying paths and go through this path.
func EnsureDirMode(path string, mode fs.FileMode) errs.DomainError {
	if err := os.MkdirAll(path, mode); err != nil {
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

// ReadJSON reads the JSON document at path, decodes it into a T, runs its
// Migrate() then Validate(), and returns the value. Because T is constrained
// to Persisted, migration (legacy-version stamping) and validation
// (including the reject-newer forward-compat guard) always run — a caller
// cannot decode a persisted document without them. Call as
// ReadJSON[Manifest](path); the pointer type param P is inferred from the
// constraint.
//
// A missing or malformed file yields ReadJSONError; a document whose schema
// version is newer than this build understands yields
// errs.NewerSchemaVersionError (with path filled in).
func ReadJSON[T any, P Persisted[T]](path string) (T, errs.DomainError) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		return v, ReadJSONError{Path: path, Err: err}
	}
	if err := json.Unmarshal(data, P(&v)); err != nil {
		return v, ReadJSONError{Path: path, Err: err}
	}
	if mErr := P(&v).Migrate(); mErr != nil {
		return v, withPath(mErr, path)
	}
	if vErr := P(&v).Validate(); vErr != nil {
		return v, withPath(vErr, path)
	}
	return v, nil
}

// WriteJSON marshals v as pretty-printed JSON with a trailing newline and
// writes it to path, creating parent directories as needed. Uses the
// project-wide default 0o755/0o644 modes for repo-projected files;
// user-private state should call WriteJSONMode with 0o700/0o600 so the
// files never become world-readable on a multi-user host.
//
// v is taken by value and Migrate/Validate run on that copy before any
// filesystem side effect, so a value failing Validate is never persisted
// (and the target directory is not even created) and the caller's value is
// never mutated by version stamping.
func WriteJSON[T any, P Persisted[T]](path string, v T) errs.DomainError {
	return WriteJSONMode[T, P](path, v, 0o755, 0o644)
}

// WriteJSONMode is WriteJSON with explicit directory and file permission
// bits. The two centralized user-config files (~/.agentfiles/profiles.json
// and projects.json) hold machine-identifying paths and pass 0o700/0o600
// so no other local user can read them.
func WriteJSONMode[T any, P Persisted[T]](path string, v T, dirMode, fileMode fs.FileMode) errs.DomainError {
	data, prepErr := prepareJSON[T, P](path, v)
	if prepErr != nil {
		return prepErr
	}
	if dirErr := EnsureDirMode(filepath.Dir(path), dirMode); dirErr != nil {
		return dirErr
	}
	if err := os.WriteFile(path, data, fileMode); err != nil {
		return WriteJSONError{Path: path, Err: err}
	}
	return nil
}

// prepareJSON runs the write-side persistence pipeline on a copy of v —
// Migrate then Validate then marshal — returning the bytes to persist (with
// a trailing newline). Validation happens before any caller touches the
// filesystem, so an invalid value never reaches disk. Shared by every
// WriteJSON* variant so the invariant cannot drift between them.
func prepareJSON[T any, P Persisted[T]](path string, v T) ([]byte, errs.DomainError) {
	if mErr := P(&v).Migrate(); mErr != nil {
		return nil, withPath(mErr, path)
	}
	if vErr := P(&v).Validate(); vErr != nil {
		return nil, withPath(vErr, path)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, WriteJSONError{Path: path, Err: err}
	}
	return append(data, '\n'), nil
}

// withPath enriches an errs.NewerSchemaVersionError with the file path when
// the value's Validate() produced it without one (a value does not know
// which file it was decoded from). Any other error passes through unchanged.
func withPath(err errs.DomainError, path string) errs.DomainError {
	var nv errs.NewerSchemaVersionError
	if errors.As(err, &nv) && nv.Path == "" {
		nv.Path = path
		return nv
	}
	return err
}

// WriteJSONAtomic writes v to a same-directory temp file, fsyncs it,
// renames it into place, and fsyncs the parent directory. A crash any
// time before the rename leaves path unchanged; a crash between the
// rename and the parent fsync still leaves a durable file (the rename
// itself is atomic). Used for the two centralized user-config files
// where a partial write would strand the migration in a state where the
// next launch's presence check trips on a corrupt file.
func WriteJSONAtomic[T any, P Persisted[T]](path string, v T, dirMode, fileMode fs.FileMode) errs.DomainError {
	data, prepErr := prepareJSON[T, P](path, v)
	if prepErr != nil {
		return prepErr
	}
	dir := filepath.Dir(path)
	if dirErr := EnsureDirMode(dir, dirMode); dirErr != nil {
		return dirErr
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return WriteJSONError{Path: path, Err: err}
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return WriteJSONError{Path: path, Err: err}
	}
	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		cleanup()
		return WriteJSONError{Path: path, Err: err}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return WriteJSONError{Path: path, Err: err}
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return WriteJSONError{Path: path, Err: err}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return WriteJSONError{Path: path, Err: err}
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
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
