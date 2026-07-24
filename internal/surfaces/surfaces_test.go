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
		{".claude/../etc/passwd", false},
		{".claude/x/../../etc", false},
		{"/etc/passwd", false},
		{`..\.claude\evil`, false},
		{"../foo", false},
		{".claude/./skills/x", true},
		{"", false},
	}
	for _, c := range cases {
		if got := IsAllowed(c.target); got != c.want {
			t.Errorf("IsAllowed(%q) = %v, want %v", c.target, got, c.want)
		}
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
