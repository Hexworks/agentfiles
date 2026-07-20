package pathselector

import (
	"path/filepath"
	"strings"
)

// isAncestor reports whether ancestor strictly contains descendant. Duplicates
// the helper in internal/app/service.go on purpose: this package sits in the
// TUI layer and must not depend on the app package (dependency direction — see
// docs/guidelines/clean_architecture.md).
func isAncestor(ancestor, descendant string) bool {
	sep := string(filepath.Separator)
	return strings.HasPrefix(descendant+sep, ancestor+sep) && ancestor != descendant
}

// isUnderRoot reports whether path sits at or under root after evaluating
// symlinks on both sides. A symlink whose target escapes root is rejected
// because filepath.EvalSymlinks resolves the escape before the comparison
// happens. An empty root means "no constraint" and always returns true.
func isUnderRoot(path, root string) bool {
	if root == "" {
		return true
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolvedPath = path
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = root
	}
	resolvedPath = filepath.Clean(resolvedPath)
	resolvedRoot = filepath.Clean(resolvedRoot)
	if resolvedPath == resolvedRoot {
		return true
	}
	return isAncestor(resolvedRoot, resolvedPath)
}

// resolveAbs cleans and (optionally) symlink-expands path. When follow is
// false the returned path is only Abs+Clean — symlinks in the input are left
// intact so the caller can decide whether to reject them. When follow is true
// the path is EvalSymlinks-resolved; on failure (e.g. the target does not
// exist yet) the function falls back to the Abs+Clean form so the caller can
// still surface a meaningful error later.
func resolveAbs(path string, follow bool) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	if !follow {
		return abs
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved)
	}
	return abs
}
