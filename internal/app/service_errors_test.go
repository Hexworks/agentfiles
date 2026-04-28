package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/addamsson/agentfiles/internal/asset"
	"github.com/addamsson/agentfiles/internal/config"
)

func TestAddProject_AccumulatesUnknownAssets(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	_, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]string{"codex"}, []string{"missing-a", "missing-b"})

	if len(addErrs) == 0 {
		t.Fatal("expected error")
	}
	got := map[string]bool{}
	for _, e := range addErrs {
		var typed AssetNotFoundError
		if errors.As(e, &typed) {
			got[typed.AssetID] = true
		}
	}
	if !got["missing-a"] || !got["missing-b"] {
		t.Fatalf("expected both ids accumulated, got %v", got)
	}
}

func TestPlan_ProjectNotFoundReturnsTypedError(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	_, err := svc.Plan("personal", "does-not-exist")

	var typed ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
	if typed.ProjectID != "does-not-exist" {
		t.Fatalf("expected id preserved, got %q", typed.ProjectID)
	}
}

// TestAddProject_RejectsMixedKnownAndUnknownAssets pins the contract
// that any unknown asset id rejects the entire AddProject call — the
// project file must NOT be written.
func TestAddProject_RejectsMixedKnownAndUnknownAssets(t *testing.T) {
	root := t.TempDir()
	profilePath := filepath.Join(root, "profile")
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", profilePath); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	// Scaffold one valid asset so the selection mixes known and unknown.
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "valid", Name: "valid", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}

	_, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]string{"codex"}, []string{"valid", "missing"})

	if len(addErrs) == 0 {
		t.Fatal("expected rejection")
	}
	var notFound AssetNotFoundError
	matched := false
	for _, e := range addErrs {
		if errors.As(e, &notFound) && notFound.AssetID == "missing" {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("expected AssetNotFoundError for 'missing', got %+v", addErrs)
	}
	// Confirm the project file was not persisted.
	entries, err := os.ReadDir(filepath.Join(profilePath, config.ProjectsDirName))
	if err != nil {
		t.Fatalf("read projects dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			t.Fatalf("expected no project file, found %s", entry.Name())
		}
	}
}

// writeFileEnsureDir is shared with the project_ownership test suite as
// a small fixture helper. Kept package-local to avoid a shared test
// helper package for two callers.
func writeFileEnsureDir(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
