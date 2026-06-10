package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/registry"
)

func TestDeleteProfile_KeepFoldersRemovesRegistryEntryOnly(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.DeleteProfile("alpha", KeepFolders); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("expected folder kept, got stat err %v", err)
	}
	reg, err := svc.Registry.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(reg.Profiles) != 0 {
		t.Fatalf("expected empty registry, got %d entries", len(reg.Profiles))
	}
}

func TestDeleteProfile_DeleteFoldersRemovesBoth(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.DeleteProfile("alpha", DeleteFolders); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Fatalf("expected folder removed, stat err = %v", err)
	}
	reg, err := svc.Registry.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(reg.Profiles) != 0 {
		t.Fatalf("expected empty registry, got %d entries", len(reg.Profiles))
	}
}

func TestDeleteProfile_FolderAlreadyMissingSucceeds(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.RemoveAll(profilePath); err != nil {
		t.Fatalf("pre-remove folder: %v", err)
	}

	if err := svc.DeleteProfile("alpha", DeleteFolders); err != nil {
		t.Fatalf("delete should tolerate missing folder: %v", err)
	}

	reg, err := svc.Registry.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(reg.Profiles) != 0 {
		t.Fatalf("expected empty registry, got %d entries", len(reg.Profiles))
	}
}

func TestDeleteProfile_UnknownRefReturnsProfileNotFoundError(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))

	err := svc.DeleteProfile("does-not-exist", KeepFolders)

	var typed registry.ProfileNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProfileNotFoundError, got %T: %v", err, err)
	}
}
