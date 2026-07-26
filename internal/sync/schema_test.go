package sync

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
)

// writeLegacyState lays down a real state.json under projectPath/.agentfiles
// with the given body.
func writeLegacyState(t *testing.T, body string) string {
	t.Helper()
	projectPath := t.TempDir()
	dir := filepath.Join(projectPath, config.StateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.StateFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return projectPath
}

func TestLoadState_LegacyNoVersionStampsCurrentPreservesGeneratorVersion(t *testing.T) {
	// given a real legacy state.json: generator_version set, no schema version
	projectPath := writeLegacyState(t, `{"generator_version":"2.0.0","project_id":"p","managed_files":{}}`)

	// when loaded
	state, err := loadState(projectPath)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	// then the schema version is stamped while generator_version is untouched
	if state.Version != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", state.Version, SchemaVersion)
	}
	if state.GeneratorVersion != "2.0.0" {
		t.Fatalf("generator_version = %q, want 2.0.0 (preserved)", state.GeneratorVersion)
	}
}

func TestLoadState_RejectsNewerVersion(t *testing.T) {
	// given a state.json whose schema version is newer than this build knows
	projectPath := writeLegacyState(t, `{"version":99,"generator_version":"2.0.0","managed_files":{}}`)

	// when loaded
	_, err := loadState(projectPath)

	// then it is rejected
	var newer errs.NewerSchemaVersionError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerSchemaVersionError, got %T: %v", err, err)
	}
}
