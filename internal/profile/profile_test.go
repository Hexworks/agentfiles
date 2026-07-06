package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/project"
)

func TestInit_DoesNotScaffoldProjectsDir(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "Personal"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "projects")); !os.IsNotExist(err) {
		t.Fatalf("expected no projects/ dir, stat err = %v", err)
	}
}

func TestLoad_ReturnsEmptyProjectsMap(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "Personal"); err != nil {
		t.Fatalf("init: %v", err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Projects == nil {
		t.Fatal("expected non-nil Projects map")
	}
	if len(loaded.Projects) != 0 {
		t.Fatalf("expected empty Projects map, got %d entries", len(loaded.Projects))
	}
}

func TestScanAssets_DuplicateIDReturnsTypedError(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "Personal"); err != nil {
		t.Fatalf("init profile: %v", err)
	}
	mustWriteAsset(t, root, "skill", "review-a", `{"id":"review","name":"a","type":"skill"}`)
	mustWriteAsset(t, root, "skill", "review-b", `{"id":"review","name":"b","type":"skill"}`)

	_, err := Load(root)

	var typed DuplicateAssetIDError
	if !errors.As(err, &typed) {
		t.Fatalf("expected DuplicateAssetIDError, got %T: %v", err, err)
	}
	if typed.ID != "review" {
		t.Fatalf("expected id preserved, got %q", typed.ID)
	}
}

func TestUnselectAsset_MutatesEveryProjectInMemory(t *testing.T) {
	p := &Profile{
		Root: t.TempDir(),
		Projects: map[string]*project.Manifest{
			"alpha": {ID: "alpha", Name: "Alpha", Path: "/tmp/alpha", EnabledAgents: []string{"codex"}, SelectedAssetIDs: []string{"review"}},
			"beta":  {ID: "beta", Name: "Beta", Path: "/tmp/beta", EnabledAgents: []string{"codex"}, SelectedAssetIDs: []string{"review"}},
			"gamma": {ID: "gamma", Name: "Gamma", Path: "/tmp/gamma", EnabledAgents: []string{"codex"}, SelectedAssetIDs: []string{"other"}},
		},
	}
	mutated := p.UnselectAsset("review")
	if len(mutated) != 2 {
		t.Fatalf("expected 2 mutated projects, got %v", mutated)
	}
	if len(p.Projects["alpha"].SelectedAssetIDs) != 0 {
		t.Fatalf("expected alpha cleared, got %v", p.Projects["alpha"].SelectedAssetIDs)
	}
	if len(p.Projects["gamma"].SelectedAssetIDs) != 1 || p.Projects["gamma"].SelectedAssetIDs[0] != "other" {
		t.Fatalf("expected gamma untouched, got %v", p.Projects["gamma"].SelectedAssetIDs)
	}
}

func mustWriteAsset(t *testing.T, root, typeDir, name, manifest string) {
	t.Helper()
	dir := filepath.Join(root, config.AssetsDirName, typeDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.AssetManifestFileName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}
