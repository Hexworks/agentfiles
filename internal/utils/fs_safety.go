package utils

import (
	"path/filepath"
	"strings"
)

// IsAncestor reports whether ancestor strictly contains descendant. The
// check is purely lexical — callers that need symlink resolution should
// route both sides through ResolveAbs (or IsUnderRoot) first.
func IsAncestor(ancestor, descendant string) bool {
	sep := string(filepath.Separator)
	return strings.HasPrefix(descendant+sep, ancestor+sep) && ancestor != descendant
}

// IsUnderRoot reports whether path sits at or under root after evaluating
// symlinks on both sides. An empty root means "no constraint" and always
// returns true.
//
// The check is fail-closed: if filepath.EvalSymlinks fails on either side
// (dangling symlink, permission-denied on a path component, TOCTOU race),
// the function returns false. Callers must treat a false return as a
// containment failure — never as "probably fine".
func IsUnderRoot(path, root string) bool {
	if root == "" {
		return true
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	resolvedPath = filepath.Clean(resolvedPath)
	resolvedRoot = filepath.Clean(resolvedRoot)
	if resolvedPath == resolvedRoot {
		return true
	}
	return IsAncestor(resolvedRoot, resolvedPath)
}

// ResolveAbs cleans and (optionally) symlink-expands path. When follow is
// false the returned path is Abs+Clean only — symlinks in the input are
// left intact so the caller can decide whether to reject them. When
// follow is true the path is EvalSymlinks-resolved; if EvalSymlinks
// fails, ResolveAbs returns a PathResolutionError (fail-closed) so the
// caller does not silently proceed on a lexical path that may have
// escaped the intended root.
func ResolveAbs(path string, follow bool) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", AbsPathError{Path: path, Err: err}
	}
	abs = filepath.Clean(abs)
	if !follow {
		return abs, nil
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", PathResolutionError{Path: abs, Err: err}
	}
	return filepath.Clean(resolved), nil
}
