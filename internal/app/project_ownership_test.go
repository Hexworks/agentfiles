package app

import (
	"path/filepath"
	"testing"
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
	if _, err := svc.AddProject("first", "Repo", projectPath, []string{"codex"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject("second", "Repo2", projectPath, []string{"codex"}, nil); err == nil {
		t.Fatal("expected ownership conflict")
	}
}
