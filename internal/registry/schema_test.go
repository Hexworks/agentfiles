package registry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/errs"
)

func TestLoad_LegacyNoVersionStampsCurrent(t *testing.T) {
	// given a real registry file with no version key
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte(`{"profiles":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// when loaded
	reg, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// then the legacy sentinel is migrated to the current version
	if reg.Version != Version {
		t.Fatalf("version = %d, want %d", reg.Version, Version)
	}
}

func TestLoad_RejectsNewerVersion(t *testing.T) {
	// given a registry written by a newer build
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"profiles":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// when loaded
	_, err := NewStore(path).Load()

	// then it is rejected
	var newer errs.NewerSchemaVersionError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerSchemaVersionError, got %T: %v", err, err)
	}
}
