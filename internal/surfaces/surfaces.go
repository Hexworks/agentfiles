// Package surfaces owns the managed-surface root list and the matcher that
// decides whether a target path falls inside one of those roots. Render uses
// it to gate projection targets; sync uses it to walk for delete candidates.
// Co-locating the data with the rule keeps both halves of the safety fence in
// one place.
package surfaces

import (
	"path/filepath"
	"slices"
	"strings"
)

// roots are the bare top-level paths that render and sync are permitted to
// touch in a target repository. Bare names — no trailing slash — because
// IsAllowed appends "/" when building the prefix form.
var roots = []string{
	"AGENTS.md",
	".claude",
	".cursor",
	".codex",
	".opencode",
	".mcp.json",
}

// Roots returns a copy of the managed-surface root list. Callers receive a
// fresh slice so mutation cannot widen the safety fence.
func Roots() []string {
	return slices.Clone(roots)
}

// IsAllowed reports whether target sits inside one of the managed-surface
// roots. A target is allowed if it equals a root exactly (e.g. "AGENTS.md")
// or sits inside one (e.g. ".claude/settings.local.json").
//
// FIX: task#0007 — does not reject ".." segments. A target like
// ".claude/../../etc/passwd" passes the prefix check today; tighten by
// cleaning the target and rejecting any escape from the managed root.
func IsAllowed(target string) bool {
	target = filepath.ToSlash(target)
	for _, root := range roots {
		if target == root || strings.HasPrefix(target, root+"/") {
			return true
		}
	}
	return false
}
