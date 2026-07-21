// Package surfaces owns two nested safety fences and the matchers that
// enforce them. The outer fence is the managed-surface root list — the
// set of top-level paths render is allowed to project into and sync is
// allowed to walk for delete candidates. The inner fence is the tighter
// asset-container-root list, a strict subset of the outer fence whose
// direct child folders are eligible for folder-based asset registration.
// Co-locating the data with the rule keeps every fence in one place; no
// caller needs to duplicate the constants or reimplement the predicate.
package surfaces

import (
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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

// cursorCommandsRoot is Cursor's folder-shaped asset container. Cursor
// flattens each skill to a single .md file inside this folder rather
// than producing one child folder per skill, but the container itself
// still acts as a registration root.
const cursorCommandsRoot = ".cursor/commands"

// skillContainerRoots is the per-agent map of skill container
// directories under which the render pipeline projects one child folder
// per skill. Kept separate from cursorCommandsRoot because Cursor uses a
// flat one-file-per-skill layout instead of folder-per-skill. Accessed
// only through SkillRoot so external packages cannot mutate the map.
var skillContainerRoots = map[string]string{
	"codex":       ".codex/skills",
	"claude-code": ".claude/skills",
	"opencode":    ".opencode/skills",
}

// SkillRoot returns the skill container directory for the given agent
// name — the folder under which the render pipeline projects one child
// folder per skill. Returns ok == false when the agent has no
// folder-per-skill container (Cursor, which flattens skills into
// single files under CursorCommandsRoot).
func SkillRoot(agent string) (string, bool) {
	root, ok := skillContainerRoots[agent]
	return root, ok
}

// CursorCommandsRoot returns the folder Cursor treats as its command
// container. Each Cursor-projected skill lands as a single .md file
// directly under this folder rather than as a child folder.
func CursorCommandsRoot() string {
	return cursorCommandsRoot
}

// assetContainerRootsSlice memoizes the sorted asset-container-root
// slice so AssetContainerRoots does not reallocate + sort per call.
var assetContainerRootsSlice = sync.OnceValue(func() []string {
	out := make([]string, 0, len(skillContainerRoots)+1)
	for _, root := range skillContainerRoots {
		out = append(out, root)
	}
	out = append(out, cursorCommandsRoot)
	slices.Sort(out)
	return out
})

// assetContainerRootSet memoizes the O(1) membership set used by
// IsAssetContainerRoot and RegisterableFolders.
var assetContainerRootSet = sync.OnceValue(func() map[string]struct{} {
	src := assetContainerRootsSlice()
	out := make(map[string]struct{}, len(src))
	for _, r := range src {
		out[r] = struct{}{}
	}
	return out
})

// AssetContainerRoots returns the fixed set of managed-surface paths
// under which a direct child folder is eligible for asset registration.
// These are the only folder-shaped asset containers; agents_doc and
// settings render to single files and have no child folder to register.
// Returns a fresh sorted slice; the caller may mutate it.
func AssetContainerRoots() []string {
	return slices.Clone(assetContainerRootsSlice())
}

// IsAssetContainerRoot reports whether p is exactly one of the
// asset-container roots (see AssetContainerRoots). O(1) map lookup;
// prefer this predicate over rebuilding a set from AssetContainerRoots
// on the hot path.
func IsAssetContainerRoot(p string) bool {
	_, ok := assetContainerRootSet()[p]
	return ok
}

// Leaf is one leaf-path classification consumed by RegisterableFolders
// and ClassifyFolderRejection. Path is the forward-slash
// project-relative key of a plan entry; IsUnknown flags the plan's
// classification as unknown (untracked file inside a managed surface).
// The two-field shape lets surfaces stay free of the app/sync change
// vocabulary — callers copy from their own FileChange values.
type Leaf struct {
	Path      string
	IsUnknown bool
}

// RegisterableFolders returns the set of directory keys eligible for
// asset registration under the tightened rule: a folder qualifies iff
// its parent path is a known asset-container root
// (IsAssetContainerRoot) and every leaf under it is IsUnknown.
//
// The implementation walks each leaf up from its immediate parent
// until it finds an ancestor whose own parent is a container root; that
// ancestor is the "direct child of a container root" candidate. Every
// leaf therefore contributes at most one tally, and folders nested
// deeper than a direct child of a container root (or anchored above
// one) are never returned.
func RegisterableFolders(leaves []Leaf) map[string]bool {
	type tally struct{ total, unknown int }
	tallies := map[string]tally{}
	for _, leaf := range leaves {
		dir := path.Dir(leaf.Path)
		for {
			parent := path.Dir(dir)
			if IsAssetContainerRoot(parent) {
				t := tallies[dir]
				t.total++
				if leaf.IsUnknown {
					t.unknown++
				}
				tallies[dir] = t
				break
			}
			if parent == "." || parent == dir {
				break
			}
			dir = parent
		}
	}
	out := map[string]bool{}
	for dir, t := range tallies {
		if t.total > 0 && t.unknown == t.total {
			out[dir] = true
		}
	}
	return out
}

// FolderRejectionReason classifies why RegisterableFolders excluded a
// specific dirKey, so callers rejecting a CreateAssetFromFolder call
// (or a stale ignored_paths entry) can render targeted messages
// instead of the generic "folder not registerable".
type FolderRejectionReason string

// FolderRejectionReason values. Set on
// FolderNotRegisterableError.Reason at emission and inspected by the
// TUI to pick a specific corrective message.
const (
	// ReasonNotUnderContainerRoot fires when dirKey's parent path is
	// not a known asset-container root — the folder sits above a
	// container root, is nested deeper than a direct child of one, or
	// lives outside every asset root entirely.
	ReasonNotUnderContainerRoot FolderRejectionReason = "not_under_container_root"
	// ReasonHasManagedDescendants fires when dirKey is a valid direct
	// child of a container root but at least one descendant leaf is
	// already managed by agentfiles (create/update/drift/delete rather
	// than unknown). Registering the folder would clobber managed
	// files.
	ReasonHasManagedDescendants FolderRejectionReason = "has_managed_descendants"
	// ReasonAbsentFromPlan fires when dirKey is a valid direct child
	// of a container root but no leaf under it appears in the current
	// plan — the folder is empty, was deleted between the TUI listing
	// and the apply, or the caller invented a stale key.
	ReasonAbsentFromPlan FolderRejectionReason = "absent_from_plan"
)

// ClassifyFolderRejection identifies which of the three rejection
// modes excludes dirKey from RegisterableFolders(leaves). Call this
// only when RegisterableFolders(leaves)[dirKey] returned false;
// classifying a registerable key is undefined.
func ClassifyFolderRejection(dirKey string, leaves []Leaf) FolderRejectionReason {
	if !IsAssetContainerRoot(path.Dir(dirKey)) {
		return ReasonNotUnderContainerRoot
	}
	prefix := dirKey + "/"
	total := 0
	for _, leaf := range leaves {
		if !strings.HasPrefix(leaf.Path, prefix) {
			continue
		}
		total++
		if !leaf.IsUnknown {
			return ReasonHasManagedDescendants
		}
	}
	if total == 0 {
		return ReasonAbsentFromPlan
	}
	return ReasonHasManagedDescendants
}
