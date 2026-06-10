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

func TestUnselectAsset_RemovesIDFromEveryProject(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "Personal"); err != nil {
		t.Fatalf("init: %v", err)
	}
	mustWriteAsset(t, root, "skill", "review", `{"id":"review","name":"review","type":"skill"}`)
	mustWriteProject(t, root, "alpha", "Alpha", "/tmp/alpha", []string{"review"})
	mustWriteProject(t, root, "beta", "Beta", "/tmp/beta", []string{"review"})
	mustWriteProject(t, root, "gamma", "Gamma", "/tmp/gamma", []string{"other"})
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := loaded.UnselectAsset("review"); err != nil {
		t.Fatalf("unselect: %v", err)
	}

	reloaded, err := Load(root)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	for _, id := range []string{"alpha", "beta"} {
		p := reloaded.Projects[id]
		if p == nil {
			t.Fatalf("project %s missing", id)
		}
		if len(p.SelectedAssetIDs) != 0 {
			t.Fatalf("project %s: expected cleared, got %v", id, p.SelectedAssetIDs)
		}
	}
	gamma := reloaded.Projects["gamma"]
	if gamma == nil || len(gamma.SelectedAssetIDs) != 1 || gamma.SelectedAssetIDs[0] != "other" {
		t.Fatalf("expected gamma untouched, got %v", gamma)
	}
}

func mustWriteProject(t *testing.T, root, id, name, path string, assetIDs []string) {
	t.Helper()
	assets := `[]`
	if len(assetIDs) > 0 {
		assets = `["` + assetIDs[0] + `"]`
		for _, a := range assetIDs[1:] {
			assets = assets[:len(assets)-1] + `,"` + a + `"]`
		}
	}
	body := `{"id":"` + id + `","name":"` + name + `","path":"` + path +
		`","enabled_agents":["codex"],"selected_asset_ids":` + assets + `}`
	mustWriteFile(t, filepath.Join(root, config.ProjectsDirName, id+".json"), body)
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
