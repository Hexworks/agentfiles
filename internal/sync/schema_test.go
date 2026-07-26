package sync

import (
	"encoding/json"
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

func TestApply_RewriteRestampsGeneratorVersionOnDisk(t *testing.T) {
	// given a real state.json whose generator_version differs from this
	// build's constant and whose schema version is the legacy sentinel. A
	// distinct seed is what makes this meaningful: if it equalled the current
	// constant, a clobber on the write path would be invisible.
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	dir := filepath.Join(projectRoot, config.StateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"generator_version":"1.5.0","profile_id":"personal","project_id":"app","managed_files":{}}`
	statePath := filepath.Join(dir, config.StateFileName)
	if err := os.WriteFile(statePath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	// when the state re-write path (Apply) runs
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	// then the on-disk bytes carry the stamped schema version and a
	// generator_version re-stamped to this build's constant. Decision C
	// (task 0001) keeps generator_version meaning "the entry format this
	// writer produced", so Apply intentionally re-stamps it rather than
	// preserving the loaded "1.5.0" — reading the file back (not the
	// in-memory struct) verifies the write side of the "keep both fields"
	// invariant.
	data, rerr := os.ReadFile(statePath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	var onDisk struct {
		Version          int    `json:"version"`
		GeneratorVersion string `json:"generator_version"`
	}
	if uerr := json.Unmarshal(data, &onDisk); uerr != nil {
		t.Fatal(uerr)
	}
	if onDisk.Version != SchemaVersion {
		t.Fatalf("on-disk version = %d, want %d", onDisk.Version, SchemaVersion)
	}
	if onDisk.GeneratorVersion != GeneratorVersion {
		t.Fatalf("on-disk generator_version = %q, want %q (re-stamped, not preserved)",
			onDisk.GeneratorVersion, GeneratorVersion)
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
