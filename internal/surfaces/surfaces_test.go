package surfaces

import "testing"

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
