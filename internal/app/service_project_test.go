package app

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
)

func TestLoadProject_ReturnsManifest(t *testing.T) {
	root := t.TempDir()
	svc := newSvc(root)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]config.Agent{config.AgentCodex}, nil); len(addErrs) > 0 {
		t.Fatalf("add: %v", addErrs)
	}

	p, err := svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Name != "Repo" {
		t.Fatalf("expected Repo, got %q", p.Name)
	}
}

func TestLoadProject_MissingReturnsProjectNotFoundError(t *testing.T) {
	root := t.TempDir()
	svc := newSvc(root)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	_, err := svc.LoadProject("personal", "missing")

	var typed ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
	if typed.ProjectID != "missing" {
		t.Fatalf("expected id preserved, got %q", typed.ProjectID)
	}
}

func TestUpdateProject_PersistsChanges(t *testing.T) {
	root := t.TempDir()
	svc := newSvc(root)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]config.Agent{config.AgentCodex}, nil); len(addErrs) > 0 {
		t.Fatalf("add: %v", addErrs)
	}
	p, err := svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := svc.UpdateProject("personal", p.ID, p.Name, p.Path, []config.Agent{config.AgentCodex, config.AgentClaudeCode}); err != nil {
		t.Fatalf("update: %v", err)
	}

	reloaded, err := svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.EnabledAgents) != 2 {
		t.Fatalf("expected 2 agents, got %v", reloaded.EnabledAgents)
	}
}

func TestDeleteProject_RemovesFromProjectStore(t *testing.T) {
	root := t.TempDir()
	svc := newSvc(root)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]config.Agent{config.AgentCodex}, nil); len(addErrs) > 0 {
		t.Fatalf("add: %v", addErrs)
	}

	if err := svc.DeleteProject("personal", "repo"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	stored, err := svc.Projects.ListByProfile("personal")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("expected empty project group, got %d", len(stored))
	}
}
