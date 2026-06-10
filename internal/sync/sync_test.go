package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/utils"
)

// agentsDocBody is the canonical desired body for the "base" agents_doc asset
// used across the test setup.
const agentsDocBody = "wanted"

// setupProfileAndProject scaffolds a profile with a single agents_doc/base
// asset producing AGENTS.md, and returns the loaded profile plus a project
// manifest pointing at projectRoot. The project enables the codex agent and
// selects the base asset.
func setupProfileAndProject(t *testing.T, projectRoot string) (*profile.Profile, *project.Manifest) {
	t.Helper()
	profileRoot := t.TempDir()
	if _, err := profile.Init(profileRoot, "Personal"); err != nil {
		t.Fatal(err)
	}
	assetDir := filepath.Join(profileRoot, config.AssetsDirName, "agents_doc", "base")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, config.AssetManifestFileName), []byte(`{"id":"base","name":"base","type":"agents_doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	return loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             projectRoot,
		EnabledAgents:    []string{"codex"},
		SelectedAssetIDs: []string{"base"},
	}
}

// writeState persists a ManagedState snapshot under the project's
// .agentfiles/state.json so subsequent Plan calls treat the project as a
// non-first-apply.
func writeState(t *testing.T, projectRoot string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.StateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	state := ManagedState{
		ProfileID:        "personal",
		ProjectID:        "app",
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    time.Now(),
		ManagedFiles:     files,
	}
	if err := os.WriteFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName), mustJSON(t, state), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findChange returns the FileChange entry for path or fails the test.
func findChange(t *testing.T, changes []FileChange, path string) FileChange {
	t.Helper()
	for _, c := range changes {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("no change for %q in %+v", path, changes)
	return FileChange{}
}

// hasChangeOfKind reports whether any FileChange in changes has the given kind.
func hasChangeOfKind(changes []FileChange, kind ChangeKind) bool {
	for _, c := range changes {
		if c.Kind == kind {
			return true
		}
	}
	return false
}

func TestPlan_FirstApply_EmitsCreateAndIgnoresStrayFiles(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "old.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	create := findChange(t, preview.Changes, "AGENTS.md")
	if create.Kind != ChangeCreate {
		t.Fatalf("AGENTS.md kind = %q, want create", create.Kind)
	}
	if create.Reason != "first apply" {
		t.Fatalf("AGENTS.md reason = %q, want \"first apply\"", create.Reason)
	}
	if hasChangeOfKind(preview.Changes, ChangeUnknown) {
		t.Fatalf("first apply must not emit ChangeUnknown: %+v", preview.Changes)
	}
	if hasChangeOfKind(preview.Changes, ChangeDelete) {
		t.Fatalf("first apply must not emit ChangeDelete: %+v", preview.Changes)
	}
}

func TestPlan_FirstApply_OverwritesPreExistingDesiredFile(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("squat"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	create := findChange(t, preview.Changes, "AGENTS.md")
	if create.Kind != ChangeCreate {
		t.Fatalf("AGENTS.md kind = %q, want create (first-apply overwrite)", create.Kind)
	}
}

func TestPlan_SubsequentApply_StateRecordedDelete(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	agentsHash := hashOf(agentsDocBody)
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{
		"AGENTS.md":      agentsHash,
		".codex/old.txt": hashOf("old"),
	})

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	deleted := findChange(t, preview.Changes, ".codex/old.txt")
	if deleted.Kind != ChangeDelete {
		t.Fatalf(".codex/old.txt kind = %q, want delete", deleted.Kind)
	}
}

func TestPlan_SubsequentApply_UnknownFile(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{
		"AGENTS.md": hashOf(agentsDocBody),
	})

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	unknown := findChange(t, preview.Changes, ".codex/stray.txt")
	if unknown.Kind != ChangeUnknown {
		t.Fatalf(".codex/stray.txt kind = %q, want unknown", unknown.Kind)
	}
	if hasChangeOfKind(preview.Changes, ChangeDelete) {
		t.Fatalf("unknown-only fixture must not emit ChangeDelete: %+v", preview.Changes)
	}
}

func TestPlan_SubsequentApply_DriftDetected(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{
		"AGENTS.md": "previous",
	})

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	drift := findChange(t, preview.Changes, "AGENTS.md")
	if drift.Kind != ChangeDrift {
		t.Fatalf("AGENTS.md kind = %q, want drift", drift.Kind)
	}
}

func TestApply_ResolveKeep_LeavesDriftAlone(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{"AGENTS.md": "previous"})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, []FileResolution{{Path: "AGENTS.md", Resolution: ResolveKeep}}); err != nil {
		t.Fatal(err)
	}

	got, readErr := os.ReadFile(filepath.Join(projectRoot, "AGENTS.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "drifted" {
		t.Fatalf("AGENTS.md = %q, want unchanged \"drifted\"", string(got))
	}
}

func TestApply_ResolveOverwrite_RewritesDrift(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{"AGENTS.md": "previous"})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, []FileResolution{{Path: "AGENTS.md", Resolution: ResolveOverwrite}}); err != nil {
		t.Fatal(err)
	}

	got, readErr := os.ReadFile(filepath.Join(projectRoot, "AGENTS.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != agentsDocBody {
		t.Fatalf("AGENTS.md = %q, want overwritten %q", string(got), agentsDocBody)
	}
}

func TestApply_DefaultUnknown_LeavesAlone(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	strayPath := filepath.Join(projectRoot, ".codex", "stray.txt")
	if err := os.WriteFile(strayPath, []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, nil); err != nil {
		t.Fatal(err)
	}

	if _, statErr := os.Stat(strayPath); statErr != nil {
		t.Fatalf("expected stray file to remain, got: %v", statErr)
	}
}

func TestApply_ResolveDelete_RemovesUnknown(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	strayPath := filepath.Join(projectRoot, ".codex", "stray.txt")
	if err := os.WriteFile(strayPath, []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, []FileResolution{{Path: ".codex/stray.txt", Resolution: ResolveDelete}}); err != nil {
		t.Fatal(err)
	}

	if _, statErr := os.Stat(strayPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected stray file removed, stat error: %v", statErr)
	}
}

func TestApply_StateDeleteRemovesFileAndDropsEntry(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	deletedPath := filepath.Join(projectRoot, ".codex", "old.txt")
	if err := os.WriteFile(deletedPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{
		"AGENTS.md":      hashOf(agentsDocBody),
		".codex/old.txt": hashOf("old"),
	})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, nil); err != nil {
		t.Fatal(err)
	}

	if _, statErr := os.Stat(deletedPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected state-recorded file removed, stat error: %v", statErr)
	}
	state := readState(t, projectRoot)
	if _, ok := state.ManagedFiles[".codex/old.txt"]; ok {
		t.Fatalf("state should not retain deleted entry: %+v", state.ManagedFiles)
	}
	if _, ok := state.ManagedFiles["AGENTS.md"]; !ok {
		t.Fatalf("state should retain still-desired entry: %+v", state.ManagedFiles)
	}
}

// readState reads and decodes the project's managed state file.
func readState(t *testing.T, projectRoot string) ManagedState {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName))
	if err != nil {
		t.Fatal(err)
	}
	var state ManagedState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

// hashOf computes the same SHA256-hex hash that utils.HashBytes produces, so
// fixtures can stamp matching values into ManagedState.
func hashOf(s string) string {
	return utils.HashBytes([]byte(s))
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
