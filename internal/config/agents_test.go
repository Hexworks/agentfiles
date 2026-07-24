package config

import (
	"slices"
	"testing"
)

func TestToAgents_AgentStrings_RoundTrip(t *testing.T) {
	ss := []string{"codex", "claude-code", "cursor", "opencode"}
	if got := AgentStrings(ToAgents(ss)); !slices.Equal(got, ss) {
		t.Fatalf("round-trip = %v, want %v", got, ss)
	}
}

func TestToAgents_AgentStrings_NilSafe(t *testing.T) {
	if got := ToAgents(nil); got != nil {
		t.Fatalf("ToAgents(nil) = %v, want nil", got)
	}
	if got := AgentStrings(nil); got != nil {
		t.Fatalf("AgentStrings(nil) = %v, want nil", got)
	}
}

func TestIsKnownAgent(t *testing.T) {
	for _, a := range AllAgents() {
		if !IsKnownAgent(a) {
			t.Fatalf("expected %q known", a)
		}
	}
	if IsKnownAgent(Agent("bogus")) {
		t.Fatal("expected bogus unknown")
	}
}
