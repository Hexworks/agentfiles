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

func TestActions_LoadAsset_ReturnsExisting(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	got, err := f.A.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "agents"})
	if err != nil {
		t.Fatalf("LoadAsset: %v", err)
	}
	if got.ID != "agents" {
		t.Fatalf("expected id agents, got %q", got.ID)
	}
}

func TestActions_LoadAsset_MissingReturnsAssetNotFoundError(t *testing.T) {
	f := newFixture(t, withProfile())

	_, err := f.A.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed app.AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_CreateAsset_ReturnsAssetDir(t *testing.T) {
	f := newFixture(t, withProfile())

	dir, err := f.A.CreateAsset(actions.CreateAssetInput{
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
	f := newFixture(t, withProfile())
	if _, err := f.A.CreateAsset(actions.CreateAssetInput{
		ProfileRef: "personal",
		Manifest:   asset.Manifest{ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := f.A.CreateAsset(actions.CreateAssetInput{
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
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	loaded, err := f.A.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "agents"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Manifest.Description = "edited"

	if _, updErr := f.A.UpdateAsset(actions.UpdateAssetInput{
		ProfileRef: "personal",
		Manifest:   &loaded.Manifest,
	}); updErr != nil {
		t.Fatalf("UpdateAsset: %v", updErr)
	}

	reloaded, _ := f.A.LoadAsset(actions.LoadAssetInput{ProfileRef: "personal", AssetID: "agents"})
	if reloaded.Description != "edited" {
		t.Fatalf("expected description edited, got %q", reloaded.Description)
	}
}

func TestActions_SelectAsset_AddsToProject(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, addErrs := f.Svc.AddProject("personal", "Repo", filepath.Join(f.Root, "repo"),
		[]string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	if _, err := f.A.SelectAsset(actions.SelectAssetInput{
		ProfileRef: "personal", ProjectID: "repo", AssetID: "agents",
	}); err != nil {
		t.Fatalf("SelectAsset: %v", err)
	}

	p, err := f.Svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if len(p.SelectedAssetIDs) != 1 || p.SelectedAssetIDs[0] != "agents" {
		t.Fatalf("expected SelectedAssetIDs=[agents], got %v", p.SelectedAssetIDs)
	}
}

func TestActions_SelectAsset_IdempotentOnDuplicate(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, addErrs := f.Svc.AddProject("personal", "Repo", filepath.Join(f.Root, "repo"),
		[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	if _, err := f.A.SelectAsset(actions.SelectAssetInput{
		ProfileRef: "personal", ProjectID: "repo", AssetID: "agents",
	}); err != nil {
		t.Fatalf("SelectAsset: %v", err)
	}

	p, _ := f.Svc.LoadProject("personal", "repo")
	if len(p.SelectedAssetIDs) != 1 {
		t.Fatalf("expected single entry, got %v", p.SelectedAssetIDs)
	}
}

func TestActions_SelectAsset_MissingAssetReturnsAssetNotFoundError(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErrs := f.Svc.AddProject("personal", "Repo", filepath.Join(f.Root, "repo"),
		[]string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	_, err := f.A.SelectAsset(actions.SelectAssetInput{
		ProfileRef: "personal", ProjectID: "repo", AssetID: "missing",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed app.AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_SelectAsset_MissingProjectReturnsProjectNotFoundError(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	_, err := f.A.SelectAsset(actions.SelectAssetInput{
		ProfileRef: "personal", ProjectID: "missing", AssetID: "agents",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed app.ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_UnselectAsset_RemovesFromProject(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, addErrs := f.Svc.AddProject("personal", "Repo", filepath.Join(f.Root, "repo"),
		[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	if _, err := f.A.UnselectAsset(actions.UnselectAssetInput{
		ProfileRef: "personal", ProjectID: "repo", AssetID: "agents",
	}); err != nil {
		t.Fatalf("UnselectAsset: %v", err)
	}

	p, _ := f.Svc.LoadProject("personal", "repo")
	if len(p.SelectedAssetIDs) != 0 {
		t.Fatalf("expected empty selection, got %v", p.SelectedAssetIDs)
	}
}

func TestActions_UnselectAsset_IdempotentOnMissingSelection(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, addErrs := f.Svc.AddProject("personal", "Repo", filepath.Join(f.Root, "repo"),
		[]string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	if _, err := f.A.UnselectAsset(actions.UnselectAssetInput{
		ProfileRef: "personal", ProjectID: "repo", AssetID: "agents",
	}); err != nil {
		t.Fatalf("UnselectAsset on never-selected asset: %v", err)
	}
}

func TestActions_UnselectAsset_MissingAssetReturnsAssetNotFoundError(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErrs := f.Svc.AddProject("personal", "Repo", filepath.Join(f.Root, "repo"),
		[]string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	_, err := f.A.UnselectAsset(actions.UnselectAssetInput{
		ProfileRef: "personal", ProjectID: "repo", AssetID: "missing",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed app.AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_DeleteAsset_RemovesAssetAndUnselectsFromProjects(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, err := f.Svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, addErrs := f.Svc.AddProject("personal", "Repo", filepath.Join(f.Root, "repo"),
		[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("seed project: %v", addErrs)
	}

	if _, err := f.A.DeleteAsset(actions.DeleteAssetInput{ProfileRef: "personal", AssetID: "agents"}); err != nil {
		t.Fatalf("DeleteAsset: %v", err)
	}

	loaded, _ := f.Svc.LoadProfile("personal")
	if _, present := loaded.Profile.Assets["agents"]; present {
		t.Fatal("expected asset removed from profile")
	}
	p, err := f.Svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if len(p.SelectedAssetIDs) != 0 {
		t.Fatalf("expected SelectedAssetIDs cleared, got %v", p.SelectedAssetIDs)
	}
}
