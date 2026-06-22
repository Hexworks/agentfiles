package app

import (
	"testing"

	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

// TestPreviewFromSync_ExposesIgnoredPaths pins the data-plumbing the Plan
// Project screen relies on: the persisted ignored set carried on the sync
// preview's ManagedState surfaces on the app-layer Preview so the TUI can
// render and un-ignore those folders without importing internal/sync.
func TestPreviewFromSync_ExposesIgnoredPaths(t *testing.T) {
	in := &llmsync.Preview{
		ProfileID: "alpha",
		ProjectID: "repo",
		Changes:   []llmsync.FileChange{{Path: "x.md", Kind: llmsync.ChangeCreate}},
		ManagedState: &llmsync.ManagedState{
			IgnoredPaths: []string{".codex/foo", ".cursor/bar"},
		},
	}

	got := previewFromSync(in)
	if got == nil {
		t.Fatal("previewFromSync returned nil")
	}
	want := []string{".codex/foo", ".cursor/bar"}
	if len(got.IgnoredPaths) != len(want) {
		t.Fatalf("IgnoredPaths = %v, want %v", got.IgnoredPaths, want)
	}
	for i, w := range want {
		if got.IgnoredPaths[i] != w {
			t.Fatalf("IgnoredPaths = %v, want %v", got.IgnoredPaths, want)
		}
	}
}

// TestPreviewFromSync_NilManagedStateYieldsNoIgnoredPaths covers the
// first-plan case: a project with no persisted state must not panic and must
// expose an empty ignored set.
func TestPreviewFromSync_NilManagedStateYieldsNoIgnoredPaths(t *testing.T) {
	got := previewFromSync(&llmsync.Preview{ProfileID: "alpha", ProjectID: "repo"})
	if got == nil {
		t.Fatal("previewFromSync returned nil")
	}
	if len(got.IgnoredPaths) != 0 {
		t.Fatalf("IgnoredPaths = %v, want empty", got.IgnoredPaths)
	}
}
