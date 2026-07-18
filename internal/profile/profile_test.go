package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
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

func TestLoad_ReturnsAssetsOnly(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "Personal"); err != nil {
		t.Fatalf("init: %v", err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Assets == nil {
		t.Fatal("expected non-nil Assets map")
	}
	if len(loaded.Assets) != 0 {
		t.Fatalf("expected empty Assets map, got %d entries", len(loaded.Assets))
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
