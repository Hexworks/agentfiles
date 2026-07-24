package app

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
)

func TestProjectPathCannotBeSharedAcrossProfiles(t *testing.T) {
	root := t.TempDir()
	svc := newSvc(root)
	firstProfile := filepath.Join(root, "first")
	secondProfile := filepath.Join(root, "second")
	if _, err := svc.CreateProfile("First", firstProfile); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProfile("Second", secondProfile); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("first", "Repo", projectPath, []config.Agent{config.AgentCodex}, nil); len(addErrs) > 0 {
		t.Fatalf("first add: %v", addErrs)
	}

	_, addErrs := svc.AddProject("second", "Repo2", projectPath, []config.Agent{config.AgentCodex}, nil)

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
	svc := newSvc(root)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("personal", "First", projectPath, []config.Agent{config.AgentCodex}, nil); len(addErrs) > 0 {
		t.Fatalf("first add: %v", addErrs)
	}

	_, addErrs := svc.AddProject("personal", "Second", projectPath, []config.Agent{config.AgentCodex}, nil)

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

// TestPathOwnershipIsEnforcedInsideStore proves the store aggregate
// itself rejects a foreign-path Add (belt-and-suspenders next to
// app.Service's translation layer). A test that reaches straight into
// s.Projects.Add bypasses AddProject; the store must still refuse.
func TestPathOwnershipIsEnforcedInsideStore(t *testing.T) {
	root := t.TempDir()
	svc := newSvc(root)
	if _, err := svc.CreateProfile("First", filepath.Join(root, "first")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProfile("Second", filepath.Join(root, "second")); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("first", "Owner1", projectPath, []config.Agent{config.AgentCodex}, nil); len(addErrs) > 0 {
		t.Fatalf("seed first: %v", addErrs)
	}

	_, addErrs := svc.AddProject("second", "Owner2", projectPath, []config.Agent{config.AgentCodex}, nil)
	if len(addErrs) == 0 {
		t.Fatal("expected store-level rejection")
	}
	var typed ProjectPathOwnedError
	if !errors.As(addErrs[0], &typed) {
		t.Fatalf("expected translated app.ProjectPathOwnedError, got %T: %v", addErrs[0], addErrs[0])
	}
	if typed.ProfileName != "First" || typed.ProjectName != "Owner1" {
		t.Fatalf("expected First/Owner1 as existing owner, got %+v", typed)
	}
}
