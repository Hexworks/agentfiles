package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
)

func TestLoadAsset_ReturnsExisting(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}

	a, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if a.ID != "agents" {
		t.Fatalf("expected id agents, got %q", a.ID)
	}
}

func TestLoadAsset_MissingReturnsAssetNotFoundError(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	_, err := svc.LoadAsset("personal", "missing")

	var typed AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
	if typed.AssetID != "missing" {
		t.Fatalf("expected id preserved, got %q", typed.AssetID)
	}
}

func TestUpdateAsset_OverwritesManifestOnDisk(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	a, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	a.Description = "updated"
	a.Tags = []string{"review"}
	if err := svc.UpdateAsset("personal", a); err != nil {
		t.Fatalf("update: %v", err)
	}

	reloaded, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Description != "updated" {
		t.Fatalf("expected description updated, got %q", reloaded.Description)
	}
	if len(reloaded.Tags) != 1 || reloaded.Tags[0] != "review" {
		t.Fatalf("expected tags [review], got %v", reloaded.Tags)
	}
}

func TestUpdateAsset_MissingAssetReturnsError(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	stranger := &asset.Asset{
		Manifest: asset.Manifest{ID: "stranger", Name: "stranger", Type: asset.TypeAgentsDoc},
		Dir:      filepath.Join(root, "profile", "assets", "agents_doc", "stranger"),
	}
	err := svc.UpdateAsset("personal", stranger)

	var typed AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

func TestDeleteAsset_RemovesAssetDirectory(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	dir, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	})
	if err != nil {
		t.Fatalf("init asset: %v", err)
	}

	if err := svc.DeleteAsset("personal", "agents"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("expected asset dir removed, stat err = %v", statErr)
	}
}

func TestDeleteAsset_UnselectsAssetFromProjects(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	if err := svc.DeleteAsset("personal", "agents"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	p, err := svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if len(p.SelectedAssetIDs) != 0 {
		t.Fatalf("expected SelectedAssetIDs cleared, got %v", p.SelectedAssetIDs)
	}
}
