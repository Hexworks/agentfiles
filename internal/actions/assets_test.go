package actions_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
)

// newAssetFixture seeds a profile with id "personal" and returns the
// Actions factory + the Service for direct seeding.
func newAssetFixture(t *testing.T) (*actions.Actions, *app.Service, string) {
	t.Helper()
	root := t.TempDir()
	svc := app.New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return actions.New(svc), svc, root
}

func TestActions_LoadAsset_ReturnsExisting(t *testing.T) {
	a, svc, _ := newAssetFixture(t)
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	got, err := a.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "agents"})
	if err != nil {
		t.Fatalf("LoadAsset: %v", err)
	}
	if got.ID != "agents" {
		t.Fatalf("expected id agents, got %q", got.ID)
	}
}

func TestActions_LoadAsset_MissingReturnsAssetNotFoundError(t *testing.T) {
	a, _, _ := newAssetFixture(t)

	_, err := a.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed app.AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_CreateAsset_ReturnsAssetDir(t *testing.T) {
	a, _, _ := newAssetFixture(t)

	dir, err := a.CreateAsset(actions.CreateAssetInput{
		ProfileRef: "personal",
		Manifest:   asset.Manifest{ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc},
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty asset dir")
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("expected asset dir to exist: %v", statErr)
	}
}

func TestActions_CreateAsset_DuplicateReturnsAssetExistsError(t *testing.T) {
	a, _, _ := newAssetFixture(t)
	if _, err := a.CreateAsset(actions.CreateAssetInput{
		ProfileRef: "personal",
		Manifest:   asset.Manifest{ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := a.CreateAsset(actions.CreateAssetInput{
		ProfileRef: "personal",
		Manifest:   asset.Manifest{ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc},
	})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	var typed app.AssetExistsError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetExistsError, got %T: %v", err, err)
	}
}

func TestActions_UpdateAsset_PersistsManifest(t *testing.T) {
	a, svc, _ := newAssetFixture(t)
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	loaded, err := a.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "agents"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Manifest.Description = "edited"

	if _, updErr := a.UpdateAsset(actions.UpdateAssetInput{
		ProfileRef: "personal",
		Manifest:   &loaded.Manifest,
	}); updErr != nil {
		t.Fatalf("UpdateAsset: %v", updErr)
	}

	reloaded, _ := a.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "agents"})
	if reloaded.Description != "edited" {
		t.Fatalf("expected description edited, got %q", reloaded.Description)
	}
}

func TestActions_DeleteAsset_RemovesAssetAndUnselectsFromProjects(t *testing.T) {
	a, svc, root := newAssetFixture(t)
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	if _, err := a.DeleteAsset(actions.DeleteAssetInput{ProfileRef: "personal", AssetID: "agents"}); err != nil {
		t.Fatalf("DeleteAsset: %v", err)
	}

	// Asset directory removed.
	loaded, _ := svc.LoadProfile("personal")
	if _, present := loaded.Assets["agents"]; present {
		t.Fatal("expected asset removed from profile")
	}
	// Project unselected.
	p, err := svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if len(p.SelectedAssetIDs) != 0 {
		t.Fatalf("expected SelectedAssetIDs cleared, got %v", p.SelectedAssetIDs)
	}
}
