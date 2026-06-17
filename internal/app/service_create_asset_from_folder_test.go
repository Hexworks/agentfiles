package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
)

func TestCreateAssetFromFolder_CopiesContentAndSelectsForProject(t *testing.T) {
	// given a profile with a project and an unmanaged source folder
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("real skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when the folder is registered as a new asset
	id, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "something", Type: asset.TypeSkill,
	}, source)
	if err != nil {
		t.Fatalf("CreateAssetFromFolder: %v", err)
	}

	// then the asset exists with the copied content and is selected
	if id != "something" {
		t.Fatalf("returned id = %q, want %q", id, "something")
	}
	prof, loadErr := svc.LoadProfile(profileID)
	if loadErr != nil {
		t.Fatalf("reload profile: %v", loadErr)
	}
	created := prof.Assets["something"]
	if created == nil {
		t.Fatal("expected asset 'something' in profile")
	}
	body, readErr := os.ReadFile(filepath.Join(created.Dir, "SKILL.md"))
	if readErr != nil {
		t.Fatalf("read copied content: %v", readErr)
	}
	if string(body) != "real skill\n" {
		t.Fatalf("copied content = %q, want %q", string(body), "real skill\n")
	}
	p, projErr := svc.LoadProject(profileID, projectID)
	if projErr != nil {
		t.Fatalf("reload project: %v", projErr)
	}
	if !slices.Contains(p.SelectedAssetIDs, "something") {
		t.Fatalf("expected 'something' selected, got %v", p.SelectedAssetIDs)
	}
}

func TestCreateAssetFromFolder_DuplicateIDReturnsAssetExistsError(t *testing.T) {
	// given a profile whose asset id collides with the derived id
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when a folder slugs to the existing id
	_, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "agents", Type: asset.TypeAgentsDoc,
	}, source)

	// then creation is rejected
	var typed AssetExistsError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetExistsError, got %T: %v", err, err)
	}
}

func TestCreateAssetFromFolder_MissingProjectReturnsProjectNotFoundError(t *testing.T) {
	svc, profileID, _ := seedServiceWithProjectAndAsset(t, "agents")

	_, err := svc.CreateAssetFromFolder(profileID, "missing-project", asset.Manifest{
		Name: "something", Type: asset.TypeSkill,
	}, t.TempDir())

	var typed ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
}
