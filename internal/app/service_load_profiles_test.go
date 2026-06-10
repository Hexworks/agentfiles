package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/utils"
)

func TestLoadProfiles_ReturnsRegisteredProfiles(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Alpha", filepath.Join(root, "alpha")); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := svc.CreateProfile("Beta", filepath.Join(root, "beta")); err != nil {
		t.Fatalf("create beta: %v", err)
	}

	loaded, loadErrs := svc.LoadProfiles()
	if len(loadErrs) != 0 {
		t.Fatalf("expected no errors, got %v", loadErrs)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(loaded))
	}
	names := map[string]bool{}
	for _, p := range loaded {
		names[p.Manifest.Name] = true
	}
	if !names["Alpha"] || !names["Beta"] {
		t.Fatalf("expected both profiles in result, got %v", names)
	}
}

func TestLoadProfiles_AggregatesPerProfileLoadErrors(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	alphaPath := filepath.Join(root, "alpha")
	betaPath := filepath.Join(root, "beta")
	if _, err := svc.CreateProfile("Alpha", alphaPath); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := svc.CreateProfile("Beta", betaPath); err != nil {
		t.Fatalf("create beta: %v", err)
	}
	// Corrupt the beta manifest so beta's Load fails.
	if err := os.WriteFile(filepath.Join(betaPath, config.ProfileManifestFileName), []byte("not-json"), 0o644); err != nil {
		t.Fatalf("corrupt beta: %v", err)
	}

	loaded, loadErrs := svc.LoadProfiles()

	if len(loaded) != 1 || loaded[0].Manifest.Name != "Alpha" {
		t.Fatalf("expected only Alpha loaded, got %v", loaded)
	}
	if len(loadErrs) != 1 {
		t.Fatalf("expected exactly one aggregated error, got %d: %v", len(loadErrs), loadErrs)
	}
	var typed utils.ReadJSONError
	if !errors.As(loadErrs[0], &typed) {
		t.Fatalf("expected ReadJSONError leaf, got %T: %v", loadErrs[0], loadErrs[0])
	}
}
