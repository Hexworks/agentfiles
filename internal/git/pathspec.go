package git

import (
	"path/filepath"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// Covers reports whether pathspec contains an entry that matches path.
// Paths are compared as forward-slash strings; a literal entry must
// match exactly, while a `foo/bar/**` wildcard matches any descendant of
// `foo/bar`. Used by Repo.Commit to reject unrelated staged changes.
func Covers(pathspec []string, path string) bool {
	path = strings.TrimPrefix(path, "./")
	for _, spec := range pathspec {
		spec = strings.TrimPrefix(spec, "./")
		if strings.HasSuffix(spec, "/**") {
			prefix := strings.TrimSuffix(spec, "/**")
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
			continue
		}
		if spec == path {
			return true
		}
	}
	return false
}

// toRepoRelative converts absolute pathspec entries into forward-slash
// paths rooted at r.Root. A trailing `/**` recursive marker is
// preserved. Every input path is `EvalSymlinks`-resolved so a symlinked
// profile folder (or any ancestor) cannot produce an entry that looks
// lexically inside r.Root while the canonical target sits elsewhere.
// r.Root is itself resolved in Detect, so both sides of the containment
// check are canonical. An entry outside r.Root returns
// UnrelatedStagedChangesError so the caller sees the same failure shape
// whether the offending path was pre-staged or hand-crafted.
func (r *Repo) toRepoRelative(pathspec []string) ([]string, errs.DomainError) {
	out := make([]string, 0, len(pathspec))
	var outside []string
	for _, p := range pathspec {
		recursive := strings.HasSuffix(p, "/**")
		base := strings.TrimSuffix(p, "/**")
		abs, err := filepath.Abs(base)
		if err != nil {
			outside = append(outside, p)
			continue
		}
		// Resolve symlinks on the caller's path so containment reflects
		// the canonical target rather than the lexical form. When the
		// leaf does not exist yet (a not-yet-created output file),
		// EvalSymlinks fails; walk up until it succeeds so the check
		// still catches a symlinked ancestor.
		canonical := resolveSymlinks(abs)
		rel, relErr := filepath.Rel(r.Root, canonical)
		if relErr != nil {
			outside = append(outside, p)
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			outside = append(outside, p)
			continue
		}
		if recursive {
			if rel == "." {
				rel = "**"
			} else {
				rel = rel + "/**"
			}
		}
		out = append(out, rel)
	}
	if len(outside) > 0 {
		return nil, UnrelatedStagedChangesError{Paths: outside}
	}
	return out, nil
}

// resolveSymlinks returns the canonical form of abs when the path (or
// its nearest existing ancestor) resolves cleanly. Returning abs on
// failure keeps the caller's lexical fallback usable so the containment
// check can still fail-closed via filepath.Rel.
func resolveSymlinks(abs string) string {
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	dir := filepath.Dir(abs)
	if dir == abs {
		return abs
	}
	base := filepath.Base(abs)
	return filepath.Join(resolveSymlinks(dir), base)
}

// toGitPathspec converts the semantic `foo/**` wildcard used by Covers
// into the corresponding literal directory that git's own pathspec
// grammar understands. Literal entries pass through unchanged.
func toGitPathspec(pathspec []string) []string {
	out := make([]string, 0, len(pathspec))
	for _, p := range pathspec {
		if strings.HasSuffix(p, "/**") {
			out = append(out, strings.TrimSuffix(p, "/**"))
			continue
		}
		out = append(out, p)
	}
	return out
}
