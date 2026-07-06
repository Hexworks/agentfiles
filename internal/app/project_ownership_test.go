package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/project"
)

func TestProjectPathCannotBeSharedAcrossProfiles(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	firstProfile := filepath.Join(root, "first")
	secondProfile := filepath.Join(root, "second")
	if _, err := svc.CreateProfile("First", firstProfile); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProfile("Second", secondProfile); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("first", "Repo", projectPath, []string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("first add: %v", addErrs)
	}

	_, addErrs := svc.AddProject("second", "Repo2", projectPath, []string{"codex"}, nil)

	if len(addErrs) == 0 {
		t.Fatal("expected ownership conflict")
	}
	var typed ProjectPathOwnedError
	if !errors.As(addErrs[0], &typed) {
		t.Fatalf("expected ProjectPathOwnedError, got %T: %v", addErrs[0], addErrs[0])
	}
	if typed.ProfileName != "First" || typed.ProjectName != "Repo" {
		t.Fatalf("unexpected owner metadata: %+v", typed)
	}
}

func TestProjectPathCannotBeSharedWithinSameProfile(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("personal", "First", projectPath, []string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("first add: %v", addErrs)
	}

	_, addErrs := svc.AddProject("personal", "Second", projectPath, []string{"codex"}, nil)

	if len(addErrs) == 0 {
		t.Fatal("expected ownership conflict")
	}
	var typed ProjectPathOwnedError
	if !errors.As(addErrs[0], &typed) {
		t.Fatalf("expected ProjectPathOwnedError, got %T: %v", addErrs[0], addErrs[0])
	}
	if typed.ProfileName != "Personal" || typed.ProjectName != "First" {
		t.Fatalf("unexpected owner metadata: %+v", typed)
	}
	if typed.Path != projectPath {
		t.Fatalf("expected path %q, got %q", projectPath, typed.Path)
	}
}

// TestEnsureProjectPathAvailable_AccumulatesAcrossMultipleProfiles
// ensures that when the same path is owned by projects in two different
// profiles, AddProject (called from a third profile) reports both
// owners in a single error slice.
func TestEnsureProjectPathAvailable_AccumulatesAcrossMultipleProfiles(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("First", filepath.Join(root, "first")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProfile("Second", filepath.Join(root, "second")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProfile("Third", filepath.Join(root, "third")); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("first", "Owner1", projectPath, []string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("first add: %v", addErrs)
	}
	if _, addErrs := svc.AddProject("second", "Owner2", projectPath, []string{"codex"}, nil); len(addErrs) == 0 {
		t.Fatal("expected second add to fail because First already owns the path")
	}

	// Sidestep AddProject to seed a second owner in the "second" group so
	// the ownership check faces two conflicts at once. Writing straight
	// to the projectstore is fine here because we are testing the
	// aggregation behavior of ensureProjectPathAvailable, not AddProject.
	if err := svc.Projects.Add("second", &project.Manifest{
		ID:            "owner2",
		Name:          "Owner2",
		Path:          projectPath,
		EnabledAgents: []string{"codex"},
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed projectstore: %v", err)
	}

	_, addErrs := svc.AddProject("third", "Owner3", projectPath, []string{"codex"}, nil)
	if len(addErrs) < 2 {
		t.Fatalf("expected at least 2 conflicts, got %d: %+v", len(addErrs), addErrs)
	}
	owners := map[string]bool{}
	for _, e := range addErrs {
		var typed ProjectPathOwnedError
		if errors.As(e, &typed) {
			owners[typed.ProfileName] = true
		}
	}
	if !owners["First"] || !owners["Second"] {
		t.Fatalf("expected both profiles in conflict set, got %v", owners)
	}
}
