package agent

import (
	"slices"
	"testing"
)

func TestFromStrings_Strings_RoundTrip(t *testing.T) {
	ss := []string{"codex", "claude-code", "cursor", "opencode"}
	if got := Strings(FromStrings(ss)); !slices.Equal(got, ss) {
		t.Fatalf("round-trip = %v, want %v", got, ss)
	}
}

func TestFromStrings_Strings_NilSafe(t *testing.T) {
	if got := FromStrings(nil); got != nil {
		t.Fatalf("FromStrings(nil) = %v, want nil", got)
	}
	if got := Strings(nil); got != nil {
		t.Fatalf("Strings(nil) = %v, want nil", got)
	}
}

func TestIsKnown(t *testing.T) {
	for _, a := range All() {
		if !IsKnown(a) {
			t.Fatalf("expected %q known", a)
		}
	}
	if IsKnown(Agent("bogus")) {
		t.Fatal("expected bogus unknown")
	}
}

func TestUnknown_DedupsAndPreservesOrder(t *testing.T) {
	in := []Agent{Codex, "bogus", ClaudeCode, "bogus", "nope"}
	if got := Unknown(in); !slices.Equal(got, []Agent{"bogus", "nope"}) {
		t.Fatalf("Unknown = %v, want [bogus nope]", got)
	}
}

func TestUnknown_NilAndAllKnown(t *testing.T) {
	if got := Unknown(nil); got != nil {
		t.Fatalf("Unknown(nil) = %v, want nil", got)
	}
	if got := Unknown(All()); got != nil {
		t.Fatalf("Unknown(All()) = %v, want nil", got)
	}
}
