package asset

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
)

// ResolveRelative is the safety rail for any in-asset file mutation: it
// rejects relative paths that escape dir, name the asset manifest itself,
// or hide as dotfiles (the same names RelativeFiles refuses to surface).
// On success it returns the cleaned absolute path the caller may pass to
// the filesystem.
//
// Containment is enforced after expanding symlinks on dir and on the
// deepest existing ancestor of the target — a symlink placed inside the
// asset folder by a previous sync or an external tool cannot smuggle a
// write to /etc/passwd through this gate.
func ResolveRelative(dir, rel string) (string, errs.DomainError) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", FilePathError{Path: rel, Reason: "empty path"}
	}
	cleanRoot, err := filepath.Abs(dir)
	if err != nil {
		return "", FilePathError{Path: rel, Reason: err.Error()}
	}
	if resolved, err := filepath.EvalSymlinks(cleanRoot); err == nil {
		cleanRoot = resolved
	}
	abs := filepath.Join(cleanRoot, rel)
	resolved := resolveExisting(abs)
	rp, err := filepath.Rel(cleanRoot, resolved)
	if err != nil {
		return "", FilePathError{Path: rel, Reason: err.Error()}
	}
	if rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) {
		return "", FilePathError{Path: rel, Reason: "escapes asset folder"}
	}
	if reason, ok := reservedRel(rp); ok {
		return "", FilePathError{Path: rel, Reason: reason}
	}
	return abs, nil
}

// resolveExisting walks abs from the deepest existing ancestor back up,
// EvalSymlinks-expands that ancestor, and reattaches the missing tail.
// Symlinks on the existing prefix are expanded; the missing tail is
// preserved as-is so a brand-new file's path keeps its requested name.
func resolveExisting(abs string) string {
	clean := filepath.Clean(abs)
	tail := ""
	for {
		if _, err := os.Lstat(clean); err == nil {
			if resolved, err := filepath.EvalSymlinks(clean); err == nil {
				clean = resolved
			}
			if tail == "" {
				return clean
			}
			return filepath.Join(clean, tail)
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return abs
		}
		if tail == "" {
			tail = filepath.Base(clean)
		} else {
			tail = filepath.Join(filepath.Base(clean), tail)
		}
		clean = parent
	}
}

// reservedRel rejects file names RelativeFiles would skip on read so the
// write side stays symmetric: the asset manifest itself and any dotfile
// segment.
func reservedRel(rel string) (string, bool) {
	base := filepath.Base(rel)
	if base == config.AssetManifestFileName {
		return "manifest file is reserved", true
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") {
			return "dotfile paths are reserved", true
		}
	}
	return "", false
}

// AddFile creates an empty file at dir/rel after validating the relative
// path through ResolveRelative. Intermediate directories are created with
// 0o755; the file itself is created empty with 0o644 so subsequent edits
// (e.g. the editor flow) can grow it.
func AddFile(dir, rel string) errs.DomainError {
	abs, err := ResolveRelative(dir, rel)
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
		return FileCreateError{Path: rel, Err: mkErr}
	}
	if wErr := os.WriteFile(abs, nil, 0o644); wErr != nil {
		return FileCreateError{Path: rel, Err: wErr}
	}
	return nil
}

// RemoveFile deletes the file at dir/rel after validating the relative
// path through ResolveRelative. A pre-missing file is treated as success
// so the operation is idempotent.
func RemoveFile(dir, rel string) errs.DomainError {
	abs, err := ResolveRelative(dir, rel)
	if err != nil {
		return err
	}
	if rmErr := os.Remove(abs); rmErr != nil && !os.IsNotExist(rmErr) {
		return FileRemoveError{Path: rel, Err: rmErr}
	}
	return nil
}

// WriteFile writes body to dir/rel after validating the relative path
// through ResolveRelative. Intermediate directories are created with
// 0o755; the file itself is written with mode. Used by the Adopt path
// (ADR 0020) to push a local edit back into the profile source it
// was rendered from — same containment rail as AddFile / RemoveFile.
func WriteFile(dir, rel string, body []byte, mode os.FileMode) errs.DomainError {
	abs, err := ResolveRelative(dir, rel)
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
		return FileCreateError{Path: rel, Err: mkErr}
	}
	if wErr := os.WriteFile(abs, body, mode); wErr != nil {
		return FileCreateError{Path: rel, Err: wErr}
	}
	return nil
}

// Equal reports whether two manifests describe the same asset. Slice
// fields treat nil and an empty slice as equivalent (slices.Equal
// semantics) so a freshly-loaded manifest equals one that has been
// round-tripped through an empty form.
func (m Manifest) Equal(other Manifest) bool {
	return m.ID == other.ID &&
		m.Name == other.Name &&
		m.Type == other.Type &&
		m.Description == other.Description &&
		m.ExclusiveGroup == other.ExclusiveGroup &&
		slices.Equal(m.Tags, other.Tags) &&
		slices.Equal(m.CompatibleAgents, other.CompatibleAgents) &&
		projectionsEqual(m.Projections, other.Projections)
}

func projectionsEqual(a, b []Projection) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
