package surfaces

import (
	"slices"
	"testing"
)

func TestIsAllowed(t *testing.T) {
	cases := []struct {
		target string
		want   bool
	}{
		{"AGENTS.md", true},
		{"AGENTS.mdfoo", false},
		{"CLAUDE.md", true},
		{"CLAUDE.mdfoo", false},
		{".claude", true},
		{".claude/settings.local.json", true},
		{".claude/skills/x/SKILL.md", true},
		{".claudefoo/x", false},
		{".codex", true},
		{".codex/config.toml", true},
		{".cursor/commands/x.md", true},
		{".opencode/skills/x/y", true},
		{".mcp.json", true},
		{".mcp.jsonfoo", false},
		{"randomfile", false},
		{".agentfiles/state.json", false},
		// --- path traversal / escape rejection (task 0007) ---
		{".claude/../etc/passwd", false}, // cleans to "etc/passwd": refused by root-mismatch, not the "../" guard
		{".claude/x/../../etc", false},   // cleans to "../etc": escapes above repo root
		{"/etc/passwd", false},           // absolute path
		{`..\.claude\evil`, false},       // backslash separators: rejected before cleaning
		{"../foo", false},                // escapes above repo root
		{".claude/./skills/x", true},     // benign "." segment must survive Clean, not be over-rejected
		{".claude/../.claude/x", true},   // escape + re-enter cleans to ".claude/x": stays inside the fence
		{"..", false},                    // hits the cleaned == ".." branch directly
		{".claude/..", false},            // cleans to ".": traverses back to cwd, not a managed root
		{".", false},                     // cwd itself is not a managed surface
		{"", false},                      // empty target guarded explicitly
	}
	for _, c := range cases {
		if got := IsAllowed(c.target); got != c.want {
			t.Errorf("IsAllowed(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}

// TestSurfaces_AllowsClaudeMd pins that widening the managed-surface
// fence to CLAUDE.md (so the claude-code agents_doc target can be
// projected and synced) took effect. The render→sync integration side of
// this criterion is covered by TestPlan_ClaudeMd_SurfacesNotFenced in the
// sync package.
func TestSurfaces_AllowsClaudeMd(t *testing.T) {
	if !IsAllowed("CLAUDE.md") {
		t.Fatal("IsAllowed(\"CLAUDE.md\") = false, want true after fence widening")
	}
	if !slices.Contains(Roots(), "CLAUDE.md") {
		t.Fatal("Roots() missing CLAUDE.md")
	}
}

func TestRootsReturnsFreshCopy(t *testing.T) {
	a := Roots()
	a[0] = "mutated"
	b := Roots()
	if b[0] == "mutated" {
		t.Fatal("Roots() returned shared backing array; mutation leaked")
	}
}

// TestAssetContainerRoots pins the exact sorted set of asset-container
// roots. If this test fails, either a supported agent gained/lost a
// folder-shaped container or the sort order changed — both are
// breaking for callers that treat the slice as canonical.
func TestAssetContainerRoots(t *testing.T) {
	got := AssetContainerRoots()
	want := []string{
		".claude/skills",
		".codex/skills",
		".cursor/commands",
		".opencode/skills",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("AssetContainerRoots() = %v, want %v", got, want)
	}
}

// TestAssetContainerRootsReturnsFreshCopy pins the caller-may-mutate
// contract in the godoc: mutating the returned slice must not leak
// into subsequent calls.
func TestAssetContainerRootsReturnsFreshCopy(t *testing.T) {
	a := AssetContainerRoots()
	a[0] = "mutated"
	b := AssetContainerRoots()
	if b[0] == "mutated" {
		t.Fatal("AssetContainerRoots() returned shared backing array; mutation leaked")
	}
}

func TestIsAssetContainerRoot(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{".claude/skills", true},
		{".codex/skills", true},
		{".cursor/commands", true},
		{".opencode/skills", true},
		{".claude", false},
		{".claude/skills/foo", false},
		{"", false},
		{"docs", false},
	}
	for _, c := range cases {
		if got := IsAssetContainerRoot(c.in); got != c.want {
			t.Errorf("IsAssetContainerRoot(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRegisterableFolders(t *testing.T) {
	leaves := []Leaf{
		{Path: ".claude/skills/foo/SKILL.md", IsUnknown: true},
		{Path: ".claude/skills/foo/helper.md", IsUnknown: true},
		{Path: ".claude/skills/bar/SKILL.md", IsUnknown: true},
		{Path: ".claude/skills/bar/managed.md", IsUnknown: false},
		{Path: ".claude/skills/nest/deep/f.md", IsUnknown: true},
		{Path: "docs/whatever/notes.md", IsUnknown: true},
	}
	got := RegisterableFolders(leaves)

	for _, dir := range []string{".claude/skills/foo", ".claude/skills/nest"} {
		if !got[dir] {
			t.Errorf("RegisterableFolders()[%q] = false, want true", dir)
		}
	}
	for _, dir := range []string{
		".claude/skills/bar",
		".claude/skills/nest/deep",
		".claude/skills",
		".claude",
		"docs/whatever",
		"docs",
	} {
		if got[dir] {
			t.Errorf("RegisterableFolders()[%q] = true, want false", dir)
		}
	}
}

func TestClassifyFolderRejection(t *testing.T) {
	leaves := []Leaf{
		{Path: ".claude/skills/foo/SKILL.md", IsUnknown: true},
		{Path: ".claude/skills/bar/managed.md", IsUnknown: false},
		{Path: ".claude/skills/bar/other.md", IsUnknown: true},
	}
	cases := []struct {
		name   string
		dirKey string
		want   FolderRejectionReason
	}{
		{"above container root", ".claude", ReasonNotUnderContainerRoot},
		{"container root itself", ".claude/skills", ReasonNotUnderContainerRoot},
		{"nested one level deeper", ".claude/skills/foo/bar", ReasonNotUnderContainerRoot},
		{"outside any container root", "docs/whatever", ReasonNotUnderContainerRoot},
		{"partly-managed direct child", ".claude/skills/bar", ReasonHasManagedDescendants},
		{"empty direct child", ".claude/skills/empty", ReasonAbsentFromPlan},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyFolderRejection(c.dirKey, leaves); got != c.want {
				t.Errorf("ClassifyFolderRejection(%q) = %q, want %q", c.dirKey, got, c.want)
			}
		})
	}
}
