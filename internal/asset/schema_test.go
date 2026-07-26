package asset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
)

func TestAsset_LoadsLegacyManifestNoVersion(t *testing.T) {
	// given a real legacy asset.json with no version key on disk
	dir := t.TempDir()
	legacy := `{"id":"a1","name":"A1","type":"rule"}`
	if err := os.WriteFile(filepath.Join(dir, config.AssetManifestFileName), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	// when loaded
	a, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// then the legacy sentinel is migrated to the current version
	if a.Version != Version {
		t.Fatalf("in-memory version = %d, want %d", a.Version, Version)
	}

	// and re-saving stamps version onto disk
	if err := SaveManifest(dir, a.Manifest); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, rerr := os.ReadFile(filepath.Join(dir, config.AssetManifestFileName))
	if rerr != nil {
		t.Fatal(rerr)
	}
	var onDisk struct {
		Version int `json:"version"`
	}
	if uerr := json.Unmarshal(data, &onDisk); uerr != nil {
		t.Fatal(uerr)
	}
	if onDisk.Version != Version {
		t.Fatalf("on-disk version = %d, want %d", onDisk.Version, Version)
	}
}

func TestAsset_RejectsNewerManifest(t *testing.T) {
	// given an asset.json written by a newer build
	dir := t.TempDir()
	future := `{"version":2,"id":"a1","name":"A1","type":"rule"}`
	if err := os.WriteFile(filepath.Join(dir, config.AssetManifestFileName), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}

	// when loaded
	_, err := Load(dir)

	// then it is rejected
	var newer errs.NewerSchemaVersionError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerSchemaVersionError, got %T: %v", err, err)
	}
}
