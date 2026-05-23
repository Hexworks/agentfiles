package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
)

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

func TestScanProjects_DuplicateIDReturnsTypedError(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "Personal"); err != nil {
		t.Fatalf("init profile: %v", err)
	}
	projectDir := filepath.Join(root, config.ProjectsDirName)
	mustWriteFile(t, filepath.Join(projectDir, "a.json"),
		`{"id":"app","name":"app-a","path":"/tmp/a","enabled_agents":["codex"]}`)
	mustWriteFile(t, filepath.Join(projectDir, "b.json"),
		`{"id":"app","name":"app-b","path":"/tmp/b","enabled_agents":["codex"]}`)

	_, err := Load(root)

	var typed DuplicateProjectIDError
	if !errors.As(err, &typed) {
		t.Fatalf("expected DuplicateProjectIDError, got %T: %v", err, err)
	}
	if typed.ID != "app" {
		t.Fatalf("expected id preserved, got %q", typed.ID)
	}
}

func mustWriteAsset(t *testing.T, root, typeDir, name, manifest string) {
	t.Helper()
	dir := filepath.Join(root, config.AssetsDirName, typeDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, config.AssetManifestFileName), manifest)
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
