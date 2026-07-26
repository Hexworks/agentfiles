package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
)

// writeLegacyProfile lays down a profile root with the given profile.json
// body and an empty assets/ tree so Load's asset scan succeeds.
func writeLegacyProfile(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.AssetsDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.ProfileManifestFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoad_LegacyNoVersionStampsCurrent(t *testing.T) {
	// given a profile.json with no version key
	root := writeLegacyProfile(t, `{"id":"p1","name":"P1"}`)

	// when loaded
	p, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// then the legacy sentinel is migrated to the current version
	if p.Manifest.Version != Version {
		t.Fatalf("version = %d, want %d", p.Manifest.Version, Version)
	}
}

func TestLoad_RejectsNewerVersion(t *testing.T) {
	// given a profile.json written by a newer build
	root := writeLegacyProfile(t, `{"version":99,"id":"p1","name":"P1"}`)

	// when loaded
	_, err := Load(root)

	// then it is rejected
	var newer errs.NewerSchemaVersionError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerSchemaVersionError, got %T: %v", err, err)
	}
}
