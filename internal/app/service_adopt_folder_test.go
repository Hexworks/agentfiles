package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
)

// seedUnadoptedProfile creates a profile and drops a bare folder (no
// asset.json) under assets/<rel>/ holding the given files, mimicking a
// skill copied in by hand.
func seedUnadoptedProfile(t *testing.T, rel string, files ...string) (svc *Service, profileID, dir string) {
	t.Helper()
	root := t.TempDir()
	svc = newSvc(root)
	profilePath := filepath.Join(root, "profile")
	if _, err := svc.CreateProfile("Personal", profilePath); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	dir = filepath.Join(profilePath, config.AssetsDirName, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("# body"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return svc, "personal", dir
}

func TestListUnadoptedAssetFolders(t *testing.T) {
	svc, profileID, _ := seedUnadoptedProfile(t, "skill/copied", "SKILL.md")

	got, err := svc.ListUnadoptedAssetFolders(profileID)
	if err != nil {
		t.Fatalf("ListUnadoptedAssetFolders: %v", err)
	}
	if len(got) != 1 || got[0].Key != "skill/copied" || got[0].Type != asset.TypeSkill {
		t.Fatalf("got %+v, want one skill/copied entry", got)
	}
	loaded, loadErr := svc.LoadProfile(profileID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(loaded.Profile.Assets) != 0 {
		t.Errorf("unadopted folder must not load as an asset, got %v", loaded.Profile.Assets)
	}
}

func TestAdoptAssetFolder_MakesFolderALoadedAsset(t *testing.T) {
	svc, profileID, dir := seedUnadoptedProfile(t, "skill/copied", "SKILL.md", "helper.md")

	id, err := svc.AdoptAssetFolder(profileID, asset.Manifest{Name: "Copied", Type: asset.TypeSkill}, "skill/copied")
	if err != nil {
		t.Fatalf("AdoptAssetFolder: %v", err)
	}
	if id != "copied" {
		t.Errorf("id = %q, want slug of name", id)
	}
	loaded, loadErr := svc.LoadProfile(profileID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	a := loaded.Profile.Assets["copied"]
	if a == nil {
		t.Fatalf("adopted asset not loaded; assets = %v", loaded.Profile.Assets)
	}
	if a.Dir != dir {
		t.Errorf("asset dir = %q, want in-place %q", a.Dir, dir)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "helper.md")); statErr != nil {
		t.Errorf("content lost: %v", statErr)
	}
	remaining, scanErr := svc.ListUnadoptedAssetFolders(profileID)
	if scanErr != nil || len(remaining) != 0 {
		t.Errorf("after adopt: remaining = %+v, err = %v", remaining, scanErr)
	}
}

func TestAdoptAssetFolder_RejectsUnknownKey(t *testing.T) {
	svc, profileID, _ := seedUnadoptedProfile(t, "skill/copied", "SKILL.md")

	_, err := svc.AdoptAssetFolder(profileID, asset.Manifest{Name: "X", Type: asset.TypeSkill}, "skill/../../etc")
	var notAdoptable FolderNotAdoptableError
	if !errors.As(err, &notAdoptable) {
		t.Fatalf("err = %v, want FolderNotAdoptableError", err)
	}
}

func TestAdoptAssetFolder_RejectsExistingAssetID(t *testing.T) {
	svc, profileID, _ := seedUnadoptedProfile(t, "skill/copied", "SKILL.md")
	if _, err := svc.InitAsset(profileID, asset.Manifest{Name: "Copied", Type: asset.TypeAgentsDoc}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.AdoptAssetFolder(profileID, asset.Manifest{Name: "Copied", Type: asset.TypeSkill}, "skill/copied")
	var exists AssetExistsError
	if !errors.As(err, &exists) {
		t.Fatalf("err = %v, want AssetExistsError", err)
	}
}

func TestAdoptAssetFolder_RejectsMissingContentFile(t *testing.T) {
	svc, profileID, _ := seedUnadoptedProfile(t, "skill/copied", "README.md")

	_, err := svc.AdoptAssetFolder(profileID, asset.Manifest{Name: "Copied", Type: asset.TypeSkill}, "skill/copied")
	var missing asset.MissingContentFileError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingContentFileError", err)
	}
}
