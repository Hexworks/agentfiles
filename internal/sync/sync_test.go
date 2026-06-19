package sync

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		LastAppliedAt:    time.Now().UTC(),
		ManagedFiles:     files,
	}
	if err := os.WriteFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName), mustJSON(t, state), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeStateWithIgnored persists a ManagedState snapshot that also carries
// ignored folder keys, so Plan/Apply can be exercised against a project that
// already has persisted ignores.
func writeStateWithIgnored(t *testing.T, projectRoot string, files map[string]string, ignored []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.StateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	state := ManagedState{
		ProfileID:        "personal",
		ProjectID:        "app",
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    time.Now().UTC(),
		ManagedFiles:     files,
		IgnoredPaths:     ignored,
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

	if !preview.FirstApply {
		t.Fatalf("expected FirstApply=true, got false")
	}
	create := findChange(t, preview.Changes, "AGENTS.md")
	if create.Kind != ChangeCreate {
		t.Fatalf("AGENTS.md kind = %q, want create", create.Kind)
	}
	if create.Reason != ReasonFirstApply {
		t.Fatalf("AGENTS.md reason = %q, want ReasonFirstApply", create.Reason)
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
	if deleted.Reason != ReasonStateRecordedDelete {
		t.Fatalf(".codex/old.txt reason = %q, want ReasonStateRecordedDelete", deleted.Reason)
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
	if unknown.Reason != ReasonUnknown {
		t.Fatalf(".codex/stray.txt reason = %q, want ReasonUnknown", unknown.Reason)
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
	if drift.Reason != ReasonDriftDetected {
		t.Fatalf("AGENTS.md reason = %q, want ReasonDriftDetected", drift.Reason)
	}
}

func TestApply_DriftKeep_LeavesOnDiskAlone(t *testing.T) {
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

	if err := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftKeep}}}); err != nil {
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

func TestApply_DriftOverwrite_RewritesDrift(t *testing.T) {
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

	if err := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftOverwrite}}}); err != nil {
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

	if err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	if _, statErr := os.Stat(strayPath); statErr != nil {
		t.Fatalf("expected stray file to remain, got: %v", statErr)
	}
}

func TestApply_UnknownDelete_RemovesUnknown(t *testing.T) {
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

	if err := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: ".codex/stray.txt", Decision: UnknownDelete}}}); err != nil {
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

	if err := Apply(preview, Resolutions{}); err != nil {
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

// TestApply_StateRewritten asserts the post-Apply state.json contains
// exactly the paths the loop actually wrote or adopted, not "every file
// in preview.Files". Mixes create, delete (auto), unknown (kept) so the
// invariant from plan step 6 is pinned: kept unknowns never enter state,
// auto-deletes drop out, creates land with their rendered hash.
func TestApply_StateRewritten(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	// state-recorded file that is no longer desired → ChangeDelete (auto).
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// unknown stray file kept → not in state after apply.
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	// state seeded only with the soon-to-be-deleted file; AGENTS.md is a
	// fresh ChangeCreate.
	writeState(t, projectRoot, map[string]string{
		".codex/old.txt": hashOf("old"),
	})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	if got, want := state.ManagedFiles["AGENTS.md"], hashOf(agentsDocBody); got != want {
		t.Fatalf("AGENTS.md hash = %q, want %q", got, want)
	}
	if _, ok := state.ManagedFiles[".codex/old.txt"]; ok {
		t.Fatalf("auto-deleted entry must drop out of state: %+v", state.ManagedFiles)
	}
	if _, ok := state.ManagedFiles[".codex/stray.txt"]; ok {
		t.Fatalf("kept unknown must not enter state: %+v", state.ManagedFiles)
	}
}

// TestApply_DriftKeep_AdoptsCurrentAsBaseline pins the issue #1 semantics:
// keeping a drift records the on-disk hash in ManagedState so the next
// Plan no longer classifies the path as drift.
func TestApply_DriftKeep_AdoptsCurrentAsBaseline(t *testing.T) {
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

	if err := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftKeep}}}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	if got, want := state.ManagedFiles["AGENTS.md"], hashOf("drifted"); got != want {
		t.Fatalf("baseline hash = %q, want on-disk %q", got, want)
	}

	nextPreview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range nextPreview.Changes {
		if change.Path == "AGENTS.md" && change.Kind == ChangeDrift {
			t.Fatalf("expected AGENTS.md no longer ChangeDrift after keep+adopt; got %+v", nextPreview.Changes)
		}
	}
}

// TestApply_DefaultDrift_LeavesAlone is the symmetric counterpart to the
// default-unknown test: nil drift resolutions must default to DriftKeep,
// so the on-disk file remains untouched even without an explicit entry.
func TestApply_DefaultDrift_LeavesAlone(t *testing.T) {
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

	if err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	got, readErr := os.ReadFile(filepath.Join(projectRoot, "AGENTS.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "drifted" {
		t.Fatalf("AGENTS.md = %q, want unchanged \"drifted\" (default drift = keep)", string(got))
	}
}

// TestApply_DuplicateResolutions_LastWins pins the documented "duplicate
// paths: last entry wins" contract for both drift and unknown slices.
func TestApply_DuplicateResolutions_LastWins(t *testing.T) {
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

	// First entry says Keep, second says Overwrite — Overwrite wins.
	if err := Apply(preview, Resolutions{Drift: []DriftResolution{
		{Path: "AGENTS.md", Decision: DriftKeep},
		{Path: "AGENTS.md", Decision: DriftOverwrite},
	}}); err != nil {
		t.Fatal(err)
	}

	got, readErr := os.ReadFile(filepath.Join(projectRoot, "AGENTS.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != agentsDocBody {
		t.Fatalf("AGENTS.md = %q, want %q (last-wins should have overwritten)", string(got), agentsDocBody)
	}
}

// TestApply_UnknownResolutionPath_IsIgnored pins the changelog's
// "resolution path not in Changes is silently ignored" contract.
func TestApply_UnknownResolutionPath_IsIgnored(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: ".codex/does-not-exist.txt", Decision: UnknownDelete}}}); err != nil {
		t.Fatalf("expected no error for stray resolution path, got %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(projectRoot, "AGENTS.md")); statErr != nil {
		t.Fatalf("AGENTS.md should still exist: %v", statErr)
	}
}

// TestApply_InvalidResolutionPath_ReturnsTypedError exercises the path
// validator (option 17). Uses errors.As per the errors guideline.
func TestApply_InvalidResolutionPath_ReturnsTypedError(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	applyErr := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: "/etc/passwd", Decision: UnknownDelete}}})
	if applyErr == nil {
		t.Fatalf("expected InvalidPathError for absolute path, got nil")
	}
	var invalid InvalidPathError
	if !errors.As(applyErr, &invalid) {
		t.Fatalf("expected InvalidPathError, got %T: %v", applyErr, applyErr)
	}
	if invalid.Path != "/etc/passwd" {
		t.Fatalf("InvalidPathError.Path = %q, want /etc/passwd", invalid.Path)
	}
}

// TestPlan_CorruptStateKey_ReturnsTypedError exercises the
// loadState-side path validation (issue #3): a state.json key with
// ".." causes a StateCorruptError, surfacing via errors.As.
func TestPlan_CorruptStateKey_ReturnsTypedError(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	writeState(t, projectRoot, map[string]string{
		"../../etc/passwd": "dead",
	})

	_, err := Plan(loaded, proj)
	if err == nil {
		t.Fatalf("expected StateCorruptError, got nil")
	}
	var corrupt StateCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("expected StateCorruptError, got %T: %v", err, err)
	}
	if corrupt.Key != "../../etc/passwd" {
		t.Fatalf("StateCorruptError.Key = %q, want \"../../etc/passwd\"", corrupt.Key)
	}
}

// readState reads and decodes the project's managed state file.
// TestPlan_SuppressesUnknownUnderIgnoredPath pins the core behaviour: a file
// inside a persisted ignored folder never surfaces as ChangeUnknown, while a
// stray file outside it still does.
func TestPlan_SuppressesUnknownUnderIgnoredPath(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex", "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "ignored", "stray.txt"), []byte("under"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "other.txt"), []byte("sibling"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateWithIgnored(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)}, []string{".codex/ignored"})

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range preview.Changes {
		if c.Path == ".codex/ignored/stray.txt" {
			t.Fatalf("file under ignored folder must be suppressed, got %+v", c)
		}
	}
	sibling := findChange(t, preview.Changes, ".codex/other.txt")
	if sibling.Kind != ChangeUnknown {
		t.Fatalf(".codex/other.txt kind = %q, want unknown", sibling.Kind)
	}
}

// TestApply_UnionsAndPersistsIgnoredPaths pins that Apply keeps previously
// persisted ignores and adds the newly selected ones, deduplicated and sorted.
func TestApply_UnionsAndPersistsIgnoredPaths(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateWithIgnored(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)}, []string{".cursor/old"})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, Resolutions{IgnoredPaths: []string{".codex/new", ".cursor/old"}}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	want := []string{".codex/new", ".cursor/old"}
	if len(state.IgnoredPaths) != len(want) {
		t.Fatalf("ignored_paths = %v, want %v", state.IgnoredPaths, want)
	}
	for i, w := range want {
		if state.IgnoredPaths[i] != w {
			t.Fatalf("ignored_paths = %v, want %v (sorted, deduped)", state.IgnoredPaths, want)
		}
	}
}

// TestApply_InvalidIgnoredPath_ReturnsTypedError ensures ignored keys go
// through the same path validator as resolutions.
func TestApply_InvalidIgnoredPath_ReturnsTypedError(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	applyErr := Apply(preview, Resolutions{IgnoredPaths: []string{"/etc/passwd"}})
	var invalid InvalidPathError
	if !errors.As(applyErr, &invalid) {
		t.Fatalf("expected InvalidPathError, got %T: %v", applyErr, applyErr)
	}
}

// TestPlan_CorruptIgnoredPathInState_ReturnsTypedError ensures a tampered
// ignored key is rejected on load like a tampered managed-file key.
func TestPlan_CorruptIgnoredPathInState_ReturnsTypedError(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte(agentsDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateWithIgnored(t, projectRoot, map[string]string{"AGENTS.md": hashOf(agentsDocBody)}, []string{"../escape"})

	_, planErr := Plan(loaded, proj)
	var corrupt StateCorruptError
	if !errors.As(planErr, &corrupt) {
		t.Fatalf("expected StateCorruptError, got %T: %v", planErr, planErr)
	}
}

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

// TestIsUnderIgnored_PrefixBoundary pins the ig+"/" boundary: an ignored key
// must match itself and true descendants but never a sibling whose name it is
// merely a string prefix of. Dropping the "/" would silently over-suppress.
func TestIsUnderIgnored_PrefixBoundary(t *testing.T) {
	ignored := []string{".codex/ig"}
	cases := []struct {
		rel  string
		want bool
	}{
		{".codex/ig", true},         // exact match
		{".codex/ig/x.md", true},    // true descendant
		{".codex/ignore-me", false}, // prefix sibling, must not match
		{".cursor/other", false},    // unrelated
	}
	for _, c := range cases {
		if got := isUnderIgnored(c.rel, ignored); got != c.want {
			t.Errorf("isUnderIgnored(%q, %v) = %v, want %v", c.rel, ignored, got, c.want)
		}
	}
}

// TestApply_FirstApply_SerializesIgnoredPathsNull pins the empty-set form:
// with no managed ignores and no omitempty tag the key serializes as null
// (present, not omitted), matching managed_files' treatment of an empty map.
func TestApply_FirstApply_SerializesIgnoredPathsNull(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	data, readErr := os.ReadFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"ignored_paths": null`) {
		t.Fatalf("expected ignored_paths to serialize as null, got: %s", data)
	}
}
