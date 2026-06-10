package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/config"
)

func TestDelete_RemovesManifestFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.ProjectsDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &Manifest{
		ID:            "repo",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
		CreatedAt:     time.Now().UTC(),
	}
	if err := Save(root, manifest); err != nil {
		t.Fatalf("save: %v", err)
	}
	path := filepath.Join(root, config.ProjectsDirName, "repo.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected manifest on disk: %v", err)
	}

	if err := Delete(root, "repo"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected manifest removed, stat err = %v", err)
	}
}

func TestDelete_MissingFileIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.ProjectsDirName), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Delete(root, "never-existed"); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := Delete(root, "never-existed"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestDelete_RealFailureReturnsTypedError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based failure injection cannot run as root")
	}
	root := t.TempDir()
	projects := filepath.Join(root, config.ProjectsDirName)
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &Manifest{
		ID:            "repo",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
		CreatedAt:     time.Now().UTC(),
	}
	if err := Save(root, manifest); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Drop write on the parent so os.Remove cannot unlink.
	if err := os.Chmod(projects, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(projects, 0o755) })

	err := Delete(root, "repo")

	var typed ProjectDeleteError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectDeleteError, got %T: %v", err, err)
	}
	if typed.Unwrap() == nil {
		t.Fatal("expected wrapped os error preserved")
	}
}
