package app

import (
	"slices"
	"strings"
	"testing"

	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

// TestDesiredIgnored pins the reconciliation rule the Plan Project screen sends
// on Apply: (persisted − unignored) ∪ newlyIgnored, deduplicated and sorted.
func TestDesiredIgnored(t *testing.T) {
	got := DesiredIgnored(
		[]string{"keep", "drop"},
		[]string{"drop"},
		[]string{"live", "keep"}, // "keep" overlaps persisted → dedup
	)
	want := []string{"keep", "live"}
	if !slices.Equal(got, want) {
		t.Fatalf("DesiredIgnored = %v, want %v", got, want)
	}

	if got := DesiredIgnored([]string{"only"}, []string{"only"}, nil); got != nil {
		t.Fatalf("DesiredIgnored (all dropped) = %v, want nil", got)
	}
}

// TestAssertIgnoredRegisterable_ValidatesOnlyIncomingMinusPrior pins the
// replace-semantics gate: an already-persisted key is exempt (its folder is
// suppressed, no longer registerable), while a newly-added key must still be an
// all-unknown folder in the fresh plan.
func TestAssertIgnoredRegisterable_ValidatesOnlyIncomingMinusPrior(t *testing.T) {
	syncPreview := &llmsync.Preview{
		Changes: []llmsync.FileChange{{Path: "newdir/a.md", Kind: llmsync.ChangeUnknown}},
		ManagedState: &llmsync.ManagedState{
			IgnoredPaths: []string{"oldkey"},
		},
	}
	s := &Service{}

	// oldkey is persisted (exempt) and newdir is an all-unknown folder → ok.
	if err := s.assertIgnoredRegisterable(syncPreview, []string{"oldkey", "newdir"}); err != nil {
		t.Fatalf("assertIgnoredRegisterable rejected a valid set: %v", err)
	}

	// bogus is newly added and not registerable → rejected; oldkey stays exempt.
	err := s.assertIgnoredRegisterable(syncPreview, []string{"oldkey", "bogus"})
	if err == nil {
		t.Fatal("assertIgnoredRegisterable accepted a non-registerable new key")
	}
	if msg := err.Error(); !strings.Contains(msg, "bogus") || strings.Contains(msg, "oldkey") {
		t.Errorf("error = %q, want it to flag bogus and exempt oldkey", msg)
	}
}
