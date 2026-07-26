package projectstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/errs"
)

func TestLoad_LegacyNoVersionStampsCurrent(t *testing.T) {
	// given a real project store file with no version key
	path := filepath.Join(t.TempDir(), "projects.json")
	if err := os.WriteFile(path, []byte(`{"projects":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// when loaded (no orphan/validator seams wired)
	state, err := NewStore(path).Load(nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// then the legacy sentinel is migrated to the current version
	if state.Version != Version {
		t.Fatalf("version = %d, want %d", state.Version, Version)
	}
}

func TestLoad_RejectsNewerVersion(t *testing.T) {
	// given a store written by a newer build
	path := filepath.Join(t.TempDir(), "projects.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"projects":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// when loaded
	_, err := NewStore(path).Load(nil)

	// then it is rejected
	var newer errs.NewerSchemaVersionError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerSchemaVersionError, got %T: %v", err, err)
	}
}
