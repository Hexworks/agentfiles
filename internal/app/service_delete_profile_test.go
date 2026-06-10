package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/registry"
)

func TestDeleteProfile_RemovesRegistryEntryOnly(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.DeleteProfile("alpha"); err != nil {
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

func TestDeleteProfileWithFolder_RemovesBoth(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.DeleteProfileWithFolder("alpha"); err != nil {
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

func TestDeleteProfileWithFolder_FolderAlreadyMissingSucceeds(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.RemoveAll(profilePath); err != nil {
		t.Fatalf("pre-remove folder: %v", err)
	}

	if err := svc.DeleteProfileWithFolder("alpha"); err != nil {
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

	err := svc.DeleteProfile("does-not-exist")

	var typed registry.ProfileNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProfileNotFoundError, got %T: %v", err, err)
	}
}

func TestDeleteProfileWithFolder_RejectsPathThatLostItsManifest(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Remove the manifest so the path no longer looks like a profile root.
	if err := os.Remove(filepath.Join(profilePath, "profile.json")); err != nil {
		t.Fatalf("remove manifest: %v", err)
	}

	err := svc.DeleteProfileWithFolder("alpha")

	var typed ProfileFolderNotARootError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProfileFolderNotARootError, got %T: %v", err, err)
	}
	// Registry entry must survive since the destructive step refused.
	reg, loadErr := svc.Registry.Load()
	if loadErr != nil {
		t.Fatalf("load registry: %v", loadErr)
	}
	if len(reg.Profiles) != 1 {
		t.Fatalf("expected registry preserved, got %d entries", len(reg.Profiles))
	}
}

func TestDeleteProfileWithFolder_RejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	regPath := filepath.Join(root, "registry.json")
	svc := New(regPath)
	// Tamper-style entry pointing at the registry's own directory: a
	// recursive removal would also delete registry.json.
	if err := svc.Registry.Add(registry.ProfileRef{
		ID:   "evil",
		Name: "evil",
		Path: root,
	}); err != nil {
		t.Fatalf("add unsafe ref: %v", err)
	}

	err := svc.DeleteProfileWithFolder("evil")

	var typed UnsafeProfilePathError
	if !errors.As(err, &typed) {
		t.Fatalf("expected UnsafeProfilePathError, got %T: %v", err, err)
	}
	// Path on disk must be untouched.
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("expected registry directory preserved: %v", statErr)
	}
}

func TestDeleteProfileWithFolder_ReversesOrderOnRemoveAllFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based failure injection cannot run as root")
	}
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Make the parent directory non-writable so os.RemoveAll cannot delete
	// the profile folder; it can still descend into the folder, but the
	// final rmdir on the parent inode fails.
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	err := svc.DeleteProfileWithFolder("alpha")

	if err == nil {
		t.Fatal("expected ProfileFolderRemoveError")
	}
	var typed ProfileFolderRemoveError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProfileFolderRemoveError, got %T: %v", err, err)
	}
	if typed.Unwrap() == nil {
		t.Fatal("expected wrapped os error preserved")
	}

	// Restore permissions so the registry can be reloaded.
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatalf("restore parent perms: %v", err)
	}
	reg, loadErr := svc.Registry.Load()
	if loadErr != nil {
		t.Fatalf("load registry: %v", loadErr)
	}
	if len(reg.Profiles) != 1 {
		t.Fatalf("expected registry entry preserved on failure, got %d", len(reg.Profiles))
	}
}

func TestDeleteProfileWithFolder_DoesNotFollowSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "alpha")
	if _, err := svc.CreateProfile("Alpha", profilePath); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Create an unrelated directory and link to it from inside the
	// profile. os.RemoveAll must unlink the symlink, not delete the
	// target.
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	canary := filepath.Join(outside, "canary")
	if err := os.WriteFile(canary, []byte("preserve me"), 0o644); err != nil {
		t.Fatalf("write canary: %v", err)
	}
	linkPath := filepath.Join(profilePath, "escape")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if err := svc.DeleteProfileWithFolder("alpha"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(canary); err != nil {
		t.Fatalf("symlink target was traversed; canary missing: %v", err)
	}
}
