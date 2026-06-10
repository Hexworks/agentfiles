package actions_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/utils"
)

func newProfileFixture(t *testing.T) (*actions.Actions, *app.Service, string /*root*/) {
	t.Helper()
	root := t.TempDir()
	svc := app.New(filepath.Join(root, "registry.json"))
	return actions.New(svc), svc, root
}

func TestActions_LoadProfiles_ReturnsAll(t *testing.T) {
	a, svc, root := newProfileFixture(t)
	if _, err := svc.CreateProfile("Alpha", filepath.Join(root, "alpha")); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if _, err := svc.CreateProfile("Beta", filepath.Join(root, "beta")); err != nil {
		t.Fatalf("seed beta: %v", err)
	}

	profiles, err := a.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestActions_LoadProfiles_CollapsesErrorsIntoErrsErrors(t *testing.T) {
	a, svc, root := newProfileFixture(t)
	betaPath := filepath.Join(root, "beta")
	if _, err := svc.CreateProfile("Beta", betaPath); err != nil {
		t.Fatalf("seed beta: %v", err)
	}
	// Corrupt the manifest so Load fails.
	if err := os.WriteFile(filepath.Join(betaPath, config.ProfileManifestFileName), []byte("nope"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	profiles, err := a.LoadProfiles()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(profiles) != 0 {
		t.Fatalf("expected zero loaded profiles, got %d", len(profiles))
	}
	var aggregated errs.Errors
	if !errors.As(err, &aggregated) {
		t.Fatalf("expected errs.Errors, got %T: %v", err, err)
	}
	if len(aggregated) == 0 {
		t.Fatal("expected at least one underlying error")
	}
	var leaf utils.ReadJSONError
	if !errors.As(err, &leaf) {
		t.Fatalf("expected ReadJSONError leaf inside errs.Errors, got %v", err)
	}
}

func TestActions_LoadProfile_ResolvesByID(t *testing.T) {
	a, svc, root := newProfileFixture(t)
	ref, err := svc.CreateProfile("Alpha", filepath.Join(root, "alpha"))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, loadErr := a.LoadProfile(actions.LoadProfileInput{ProfileRef: ref.ID})
	if loadErr != nil {
		t.Fatalf("LoadProfile: %v", loadErr)
	}
	if got.Manifest.ID != ref.ID {
		t.Fatalf("expected id %q, got %q", ref.ID, got.Manifest.ID)
	}
}

func TestActions_LoadProfile_MissingReturnsProfileNotFoundError(t *testing.T) {
	a, _, _ := newProfileFixture(t)

	_, err := a.LoadProfile(actions.LoadProfileInput{ProfileRef: "ghost"})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed registry.ProfileNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProfileNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_CreateProfile_UnpacksNameAndPath(t *testing.T) {
	a, _, root := newProfileFixture(t)
	path := filepath.Join(root, "alpha")

	ref, err := a.CreateProfile(actions.CreateProfileInput{Name: "Alpha", Path: path})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if ref.Name != "Alpha" {
		t.Fatalf("expected name Alpha, got %q", ref.Name)
	}
	if ref.Path != path {
		t.Fatalf("expected path %q, got %q", path, ref.Path)
	}
	if _, statErr := os.Stat(filepath.Join(path, config.ProfileManifestFileName)); statErr != nil {
		t.Fatalf("expected profile.json on disk: %v", statErr)
	}
}

func TestActions_RegisterProfile_UnpacksPath(t *testing.T) {
	a, svc, root := newProfileFixture(t)
	// Seed by creating, then deregister via direct registry mutation so
	// RegisterProfile adopts the folder.
	path := filepath.Join(root, "alpha")
	ref, err := svc.CreateProfile("Alpha", path)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.DeleteProfile(ref.ID); err != nil {
		t.Fatalf("deregister: %v", err)
	}

	got, regErr := a.RegisterProfile(actions.RegisterProfileInput{Path: path})
	if regErr != nil {
		t.Fatalf("RegisterProfile: %v", regErr)
	}
	if got.Path != path {
		t.Fatalf("expected path %q, got %q", path, got.Path)
	}
}

func TestActions_DeleteProfile_KeepFoldersLeavesFolderOnDisk(t *testing.T) {
	a, svc, root := newProfileFixture(t)
	path := filepath.Join(root, "alpha")
	ref, err := svc.CreateProfile("Alpha", path)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, delErr := a.DeleteProfile(actions.DeleteProfileInput{ProfileRef: ref.ID, FolderAction: actions.KeepFolders}); delErr != nil {
		t.Fatalf("DeleteProfile: %v", delErr)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected folder kept, got stat err %v", statErr)
	}
}

func TestActions_DeleteProfile_DeleteFoldersRemovesFolder(t *testing.T) {
	a, svc, root := newProfileFixture(t)
	path := filepath.Join(root, "alpha")
	ref, err := svc.CreateProfile("Alpha", path)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, delErr := a.DeleteProfile(actions.DeleteProfileInput{ProfileRef: ref.ID, FolderAction: actions.DeleteFolders}); delErr != nil {
		t.Fatalf("DeleteProfile: %v", delErr)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected folder removed, stat err = %v", statErr)
	}
}

func TestActions_DeleteProfile_ReturnsStructZeroOnSuccess(t *testing.T) {
	a, svc, root := newProfileFixture(t)
	ref, err := svc.CreateProfile("Alpha", filepath.Join(root, "alpha"))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, delErr := a.DeleteProfile(actions.DeleteProfileInput{ProfileRef: ref.ID})
	if delErr != nil {
		t.Fatalf("DeleteProfile: %v", delErr)
	}
	if got != (struct{}{}) {
		t.Fatalf("expected struct{}{} return, got %v", got)
	}
}
