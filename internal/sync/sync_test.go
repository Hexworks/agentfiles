package sync

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/render"
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
		EnabledAgents:    []agent.Agent{agent.Codex},
		SelectedAssetIDs: []string{"base"},
	}
}

// writeState persists a ManagedState snapshot under the project's
// .agentfiles/state.json so subsequent Plan calls treat the project as a
// non-first-apply. Entries carry only Hash (v2-shape legacy from the
// caller's perspective), which is enough for drift/delete/unknown
// classification tests.
func writeState(t *testing.T, projectRoot string, files map[string]string) {
	t.Helper()
	writeStateEntries(t, projectRoot, hashesToEntries(files), nil)
}

// writeStateWithIgnored persists a ManagedState snapshot that also carries
// ignored folder keys, so Plan/Apply can be exercised against a project that
// already has persisted ignores.
func writeStateWithIgnored(t *testing.T, projectRoot string, files map[string]string, ignored []string) {
	t.Helper()
	writeStateEntries(t, projectRoot, hashesToEntries(files), ignored)
}

// writeStateEntries is the low-level test helper that lets a caller
// stamp v3-shape entries (with AssetID/SourceRel populated) or an
// otherwise-tuned ManagedState onto disk.
func writeStateEntries(t *testing.T, projectRoot string, entries map[string]ManagedFileEntry, ignored []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.StateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	state := ManagedState{
		ProfileID:        "personal",
		ProjectID:        "app",
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    time.Now().UTC(),
		ManagedFiles:     entries,
		IgnoredPaths:     ignored,
	}
	if err := os.WriteFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName), mustJSON(t, state), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeStateV2Legacy stamps the legacy v2 shape (managed_files entries
// serialized as bare hash strings) onto disk so the loader's
// backwards-compatible UnmarshalJSON branch is covered.
func writeStateV2Legacy(t *testing.T, projectRoot string, files map[string]string, ignored []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.StateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	type legacyState struct {
		ProfileID        string            `json:"profile_id"`
		ProjectID        string            `json:"project_id"`
		GeneratorVersion string            `json:"generator_version"`
		LastAppliedAt    time.Time         `json:"last_applied_at"`
		ManagedFiles     map[string]string `json:"managed_files"`
		IgnoredPaths     []string          `json:"ignored_paths"`
	}
	state := legacyState{
		ProfileID:        "personal",
		ProjectID:        "app",
		GeneratorVersion: "1.0.0",
		LastAppliedAt:    time.Now().UTC(),
		ManagedFiles:     files,
		IgnoredPaths:     ignored,
	}
	if err := os.WriteFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName), mustJSON(t, state), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hashesToEntries(hashes map[string]string) map[string]ManagedFileEntry {
	if hashes == nil {
		return nil
	}
	out := make(map[string]ManagedFileEntry, len(hashes))
	for k, v := range hashes {
		out[k] = ManagedFileEntry{Hash: v}
	}
	return out
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

	if _, err := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftOverwrite}}}); err != nil {
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

	if _, err := Apply(preview, Resolutions{}); err != nil {
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

	if _, err := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: ".codex/stray.txt", Decision: UnknownDelete}}}); err != nil {
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

	if _, err := Apply(preview, Resolutions{}); err != nil {
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

	if _, err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	if got, want := state.ManagedFiles["AGENTS.md"].Hash, hashOf(agentsDocBody); got != want {
		t.Fatalf("AGENTS.md hash = %q, want %q", got, want)
	}
	if _, ok := state.ManagedFiles[".codex/old.txt"]; ok {
		t.Fatalf("auto-deleted entry must drop out of state: %+v", state.ManagedFiles)
	}
	if _, ok := state.ManagedFiles[".codex/stray.txt"]; ok {
		t.Fatalf("kept unknown must not enter state: %+v", state.ManagedFiles)
	}
}

// TestApply_DriftKeep_PreservesPriorBaseline pins the ADR 0015 contract:
// keeping a drift leaves the prior managed baseline untouched (neither the
// on-disk hash nor the rendered hash is adopted), so the next Plan still
// classifies the path as ChangeDrift.
func TestApply_DriftKeep_PreservesPriorBaseline(t *testing.T) {
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

	if _, err := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftKeep}}}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	if got, want := state.ManagedFiles["AGENTS.md"].Hash, "previous"; got != want {
		t.Fatalf("baseline hash = %q, want prior %q (Keep must not adopt on-disk hash)", got, want)
	}

	nextPreview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}
	next := findChange(t, nextPreview.Changes, "AGENTS.md")
	if next.Kind != ChangeDrift {
		t.Fatalf("AGENTS.md kind after Keep = %q, want %q (kept drift must stay drift)", next.Kind, ChangeDrift)
	}
}

// TestApply_DefaultDrift_LeavesAlone is the symmetric counterpart to the
// default-unknown test: nil drift resolutions must default to DriftKeep,
// so the on-disk file remains untouched, the prior baseline is preserved,
// and the next Plan still classifies the path as ChangeDrift.
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

	if _, err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	got, readErr := os.ReadFile(filepath.Join(projectRoot, "AGENTS.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "drifted" {
		t.Fatalf("AGENTS.md = %q, want unchanged \"drifted\" (default drift = keep)", string(got))
	}

	state := readState(t, projectRoot)
	if got, want := state.ManagedFiles["AGENTS.md"].Hash, "previous"; got != want {
		t.Fatalf("baseline hash = %q, want prior %q (default Keep must preserve baseline)", got, want)
	}

	nextPreview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}
	next := findChange(t, nextPreview.Changes, "AGENTS.md")
	if next.Kind != ChangeDrift {
		t.Fatalf("AGENTS.md kind after default Keep = %q, want %q (kept drift must stay drift)", next.Kind, ChangeDrift)
	}
}

// TestApply_NoResolutions_LeavesDriftBaselineUntouched asserts that an
// Apply passing an empty Resolutions{} does not silently rebaseline
// unrelated pending drift: the drifted file's baseline stays at its
// prior recorded hash, and the next Plan still classifies it as
// ChangeDrift. Also covers that the persisted ignored key drops when
// the incoming ignored set is empty.
func TestApply_NoResolutions_LeavesDriftBaselineUntouched(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateWithIgnored(t, projectRoot, map[string]string{"AGENTS.md": "previous"}, []string{".codex/ask-matt"})
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	// Un-ignore the persisted folder by sending an empty ignored set; drift
	// row gets no resolution, so it must fall through to the preserve default.
	if _, err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	if got, want := state.ManagedFiles["AGENTS.md"].Hash, "previous"; got != want {
		t.Fatalf("baseline hash = %q, want prior %q (pure ignore-set change must not touch drift baseline)", got, want)
	}
	if state.IgnoredPaths != nil {
		t.Fatalf("ignored_paths = %v, want nil (un-ignore must drop the persisted key)", state.IgnoredPaths)
	}

	nextPreview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}
	next := findChange(t, nextPreview.Changes, "AGENTS.md")
	if next.Kind != ChangeDrift {
		t.Fatalf("AGENTS.md kind after ignore-only Apply = %q, want %q (drift must survive)", next.Kind, ChangeDrift)
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
	if _, err := Apply(preview, Resolutions{Drift: []DriftResolution{
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

	if _, err := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: ".codex/does-not-exist.txt", Decision: UnknownDelete}}}); err != nil {
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

	_, applyErr := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: "/etc/passwd", Decision: UnknownDelete}}})
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

// TestApply_ReplacesIgnoredPaths pins replace (not union) semantics: the TUI
// owns the full desired set, so Apply writes the incoming keys verbatim and a
// prior key the caller omits is dropped. Here prior .cursor/old is not in the
// incoming set, so it must disappear; .codex/new is the only survivor.
func TestApply_ReplacesIgnoredPaths(t *testing.T) {
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

	if _, err := Apply(preview, Resolutions{IgnoredPaths: []string{".codex/new"}}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	want := []string{".codex/new"}
	if len(state.IgnoredPaths) != len(want) {
		t.Fatalf("ignored_paths = %v, want %v (replace, prior dropped)", state.IgnoredPaths, want)
	}
	for i, w := range want {
		if state.IgnoredPaths[i] != w {
			t.Fatalf("ignored_paths = %v, want %v (replace, sorted, deduped)", state.IgnoredPaths, want)
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

	_, applyErr := Apply(preview, Resolutions{IgnoredPaths: []string{"/etc/passwd"}})
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

	if _, err := Apply(preview, Resolutions{}); err != nil {
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

// setupProfileAndProjectWithSkill scaffolds a profile with a "foo" skill
// asset (SKILL.md) and returns the loaded profile + a project manifest
// pointing at projectRoot that enables the claude-code agent.
func setupProfileAndProjectWithSkill(t *testing.T, projectRoot string) (*profile.Profile, *project.Manifest) {
	t.Helper()
	profileRoot := t.TempDir()
	if _, err := profile.Init(profileRoot, "Personal"); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(profileRoot, config.AssetsDirName, "skill", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, config.AssetManifestFileName), []byte(`{"id":"foo","name":"foo","type":"skill"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("skill body"), 0o644); err != nil {
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
		EnabledAgents:    []agent.Agent{agent.ClaudeCode},
		SelectedAssetIDs: []string{"foo"},
	}
}

// TestPreview_ChangeUnknown_PopulatesOwningAssetIDForKnownAsset pins
// the Plan-time reverse mapping: a stray file dropped into an existing
// skill's rendered dir surfaces on the change list with OwningAssetID
// set, so the TUI can offer Adopt.
func TestPreview_ChangeUnknown_PopulatesOwningAssetIDForKnownAsset(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProjectWithSkill(t, projectRoot)
	skillDir := filepath.Join(projectRoot, ".claude", "skills", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("skill body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "example-3.md"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{
		".claude/skills/foo/SKILL.md": hashOf("skill body"),
	})

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	unknown := findChange(t, preview.Changes, ".claude/skills/foo/example-3.md")
	if unknown.Kind != ChangeUnknown {
		t.Fatalf("kind = %q, want unknown", unknown.Kind)
	}
	if unknown.OwningAssetID != "foo" {
		t.Fatalf("OwningAssetID = %q, want foo", unknown.OwningAssetID)
	}
}

// TestPreview_ChangeUnknown_LeavesOwningAssetIDEmptyForOrphan covers the
// negative case: a stray file whose parent is a container root (not a
// per-asset projection dir) has no owner and the TUI must not offer
// Adopt.
func TestPreview_ChangeUnknown_LeavesOwningAssetIDEmptyForOrphan(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProjectWithSkill(t, projectRoot)
	skillDir := filepath.Join(projectRoot, ".claude", "skills", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("skill body"), 0o644); err != nil {
		t.Fatal(err)
	}
	orphanDir := filepath.Join(projectRoot, ".claude", "skills")
	if err := os.WriteFile(filepath.Join(orphanDir, "orphan.md"), []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState(t, projectRoot, map[string]string{
		".claude/skills/foo/SKILL.md": hashOf("skill body"),
	})

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	orphan := findChange(t, preview.Changes, ".claude/skills/orphan.md")
	if orphan.Kind != ChangeUnknown {
		t.Fatalf("kind = %q, want unknown", orphan.Kind)
	}
	if orphan.OwningAssetID != "" {
		t.Fatalf("OwningAssetID = %q, want empty (parent is a container root)", orphan.OwningAssetID)
	}
}

// TestPreview_ChangeUnknown_LeavesOwningAssetIDEmptyForAmbiguousDir
// exercises the "same rendered dir, multiple assets" branch of the
// reverse lookup: when two known assets both project into the same
// rendered directory the walker cannot pick an owner, so Adopt is not
// offered for an untracked sibling there.
func TestPreview_ChangeUnknown_LeavesOwningAssetIDEmptyForAmbiguousDir(t *testing.T) {
	plan := &render.ProjectPlan{Files: []render.RenderedFile{
		{Path: ".claude/skills/shared/a.md", AssetID: "one", SourceRel: "a.md", Agent: agent.ClaudeCode, Type: asset.TypeSkill},
		{Path: ".claude/skills/shared/b.md", AssetID: "two", SourceRel: "b.md", Agent: agent.ClaudeCode, Type: asset.TypeSkill},
	}}
	if got, _, ok := plan.ReverseLookup(".claude/skills/shared/new.md"); ok || got != "" {
		t.Fatalf("OwningAssetID for ambiguous dir = %q (ok=%v), want empty", got, ok)
	}
}

// TestDriftUnknownAdoptEnumsMirror pins the string values shared by
// the sync + appapi enum pairs so a future rename cannot silently
// desync the two layers.
func TestDriftUnknownAdoptEnumsMirror(t *testing.T) {
	if got, want := string(DriftAdopt), "adopt"; got != want {
		t.Errorf("DriftAdopt = %q, want %q", got, want)
	}
	if got, want := string(UnknownAdopt), "adopt"; got != want {
		t.Errorf("UnknownAdopt = %q, want %q", got, want)
	}
}

// TestApply_DriftAdopt_WritesProfileAndClearsDrift pins Step 5: an
// Adopt request classifies the drift row, leaves the repo file
// untouched, and returns an AdoptRequest carrying the reverse-mapping
// keys the app service needs to write the profile side. Also verifies
// that Apply's own state rewrite preserves the prior baseline, so the
// path stays drift until the profile-side write in Step 6 lands.
func TestApply_DriftAdopt_WritesProfileAndClearsDrift(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
		"AGENTS.md": {Hash: "previous", AssetID: "base", SourceRel: "AGENTS.md"},
	}, nil)

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	result, applyErr := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftAdopt}}})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if len(result.AdoptRequests) != 1 {
		t.Fatalf("AdoptRequests = %+v, want single entry", result.AdoptRequests)
	}
	req := result.AdoptRequests[0]
	if req.Path != "AGENTS.md" || req.AssetID != "base" || req.SourceRel != "AGENTS.md" {
		t.Fatalf("AdoptRequest = %+v, want {AGENTS.md base AGENTS.md}", req)
	}
	// Repo body must remain the local edit — sync does not write.
	got, readErr := os.ReadFile(filepath.Join(projectRoot, "AGENTS.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "drifted-body" {
		t.Fatalf("AGENTS.md = %q, want unchanged 'drifted-body' (sync must not write repo)", got)
	}
	// Baseline still the prior hash: the profile side has not caught
	// up yet, so the next Plan will still see drift until Step 6's
	// profile write lands and the next Plan re-hashes.
	state := readState(t, projectRoot)
	if entry := state.ManagedFiles["AGENTS.md"]; entry.Hash != "previous" {
		t.Fatalf("baseline = %+v, want prior 'previous' hash", entry)
	}
}

// TestApply_UnknownAdopt_RejectsOrphanFile pins that UnknownAdopt on a
// row without an owner surfaces AdoptUnavailableError and produces no
// AdoptRequest.
func TestApply_UnknownAdopt_RejectsOrphanFile(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProjectWithSkill(t, projectRoot)
	skillDir := filepath.Join(projectRoot, ".claude", "skills", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("skill body"), 0o644); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(projectRoot, ".claude", "skills", "orphan.md")
	if err := os.WriteFile(orphan, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
		".claude/skills/foo/SKILL.md": {Hash: hashOf("skill body"), AssetID: "foo", SourceRel: "SKILL.md"},
	}, nil)

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	result, applyErr := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: ".claude/skills/orphan.md", Decision: UnknownAdopt}}})
	if applyErr == nil {
		t.Fatalf("expected AdoptUnavailableError, got nil")
	}
	var unavailable AdoptUnavailableError
	if !errors.As(applyErr, &unavailable) {
		t.Fatalf("expected AdoptUnavailableError, got %T: %v", applyErr, applyErr)
	}
	if len(result.AdoptRequests) != 0 {
		t.Fatalf("AdoptRequests = %+v, want empty", result.AdoptRequests)
	}
	// Repo file must remain in place — Adopt does not clean unknowns.
	if _, statErr := os.Stat(orphan); statErr != nil {
		t.Fatalf("orphan removed: %v", statErr)
	}
}

// TestApply_UnknownAdopt_WritesProfileAssetFile pins the happy path:
// an unknown inside a known skill's projection dir produces an
// AdoptRequest whose SourceRel matches the file's tail below the
// projection root.
func TestApply_UnknownAdopt_WritesProfileAssetFile(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProjectWithSkill(t, projectRoot)
	skillDir := filepath.Join(projectRoot, ".claude", "skills", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("skill body"), 0o644); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(skillDir, "example-3.md")
	if err := os.WriteFile(stray, []byte("stray body"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
		".claude/skills/foo/SKILL.md": {Hash: hashOf("skill body"), AssetID: "foo", SourceRel: "SKILL.md"},
	}, nil)

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	result, applyErr := Apply(preview, Resolutions{Unknown: []UnknownResolution{{Path: ".claude/skills/foo/example-3.md", Decision: UnknownAdopt}}})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if len(result.AdoptRequests) != 1 {
		t.Fatalf("AdoptRequests = %+v, want single entry", result.AdoptRequests)
	}
	req := result.AdoptRequests[0]
	if req.AssetID != "foo" || req.SourceRel != "example-3.md" || req.Path != ".claude/skills/foo/example-3.md" {
		t.Fatalf("AdoptRequest = %+v, want {.claude/skills/foo/example-3.md foo example-3.md}", req)
	}
	if _, statErr := os.Stat(stray); statErr != nil {
		t.Fatalf("stray removed: %v", statErr)
	}
}

// TestState_LoadV2LegacyEntries_LeavesAdoptDisabled covers the v2→v3
// backwards-compatible loader path: a state.json whose managed_files
// entries are bare hash strings decodes cleanly, but the resulting
// entries carry empty AssetID/SourceRel so a follow-up DriftAdopt
// resolution surfaces AdoptUnavailableError.
func TestState_LoadV2LegacyEntries_LeavesAdoptDisabled(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateV2Legacy(t, projectRoot, map[string]string{"AGENTS.md": "previous"}, nil)

	preview, planErr := Plan(loaded, proj)
	if planErr != nil {
		t.Fatalf("Plan: %v", planErr)
	}
	if got := preview.ManagedState.ManagedFiles["AGENTS.md"]; got.Hash != "previous" || got.AssetID != "" || got.SourceRel != "" {
		t.Fatalf("v2 legacy entry = %+v, want Hash-only", got)
	}

	_, applyErr := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftAdopt}}})
	if applyErr == nil {
		t.Fatalf("expected AdoptUnavailableError for legacy v2 entry, got nil")
	}
	var unavailable AdoptUnavailableError
	if !errors.As(applyErr, &unavailable) {
		t.Fatalf("expected AdoptUnavailableError, got %T: %v", applyErr, applyErr)
	}
	if unavailable.Path != "AGENTS.md" {
		t.Fatalf("Path = %q, want AGENTS.md", unavailable.Path)
	}
}

// TestState_WriteV3_IncludesAssetIDAndSourceRel pins the on-disk v3
// shape: after a real Plan+Apply cycle each managed_files entry is a
// JSON object carrying hash, asset_id and source_rel.
func TestState_WriteV3_IncludesAssetIDAndSourceRel(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Apply(preview, Resolutions{}); err != nil {
		t.Fatal(err)
	}

	data, readErr := os.ReadFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"asset_id": "base"`) {
		t.Fatalf("state.json missing asset_id field: %s", data)
	}
	if !strings.Contains(string(data), `"source_rel": "AGENTS.md"`) {
		t.Fatalf("state.json missing source_rel field: %s", data)
	}

	state := readState(t, projectRoot)
	entry := state.ManagedFiles["AGENTS.md"]
	if entry.AssetID != "base" {
		t.Errorf("AssetID = %q, want base", entry.AssetID)
	}
	if entry.SourceRel != "AGENTS.md" {
		t.Errorf("SourceRel = %q, want AGENTS.md", entry.SourceRel)
	}
	if entry.Hash == "" {
		t.Errorf("Hash empty")
	}
}

// TestApply_DriftKeep_UpgradesV2LegacyProvenance pins the schema
// migration path for a drifted-and-kept managed file. A v2 legacy entry
// (bare hash, no AssetID/SourceRel) must gain fresh v3 provenance from
// the current rendered plan on the next Apply, while the prior baseline
// Hash is preserved so the file stays classified as drift. Regression
// for the bug where preserveDriftBaseline copied the whole prior entry,
// freezing v2 legacy state forever and blocking Adopt on the following
// plan. See ADR 0020 + task 0043.
func TestApply_DriftKeep_UpgradesV2LegacyProvenance(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateV2Legacy(t, projectRoot, map[string]string{"AGENTS.md": "previous"}, nil)

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Apply(preview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftKeep}}}); err != nil {
		t.Fatal(err)
	}

	state := readState(t, projectRoot)
	entry := state.ManagedFiles["AGENTS.md"]
	if entry.Hash != "previous" {
		t.Fatalf("Hash = %q, want prior %q (Keep must preserve baseline)", entry.Hash, "previous")
	}
	if entry.AssetID != "base" {
		t.Fatalf("AssetID = %q, want %q (Keep must upgrade legacy v2 provenance)", entry.AssetID, "base")
	}
	if entry.SourceRel != "AGENTS.md" {
		t.Fatalf("SourceRel = %q, want %q (Keep must upgrade legacy v2 provenance)", entry.SourceRel, "AGENTS.md")
	}

	nextPreview, planErr := Plan(loaded, proj)
	if planErr != nil {
		t.Fatal(planErr)
	}
	if _, adoptErr := Apply(nextPreview, Resolutions{Drift: []DriftResolution{{Path: "AGENTS.md", Decision: DriftAdopt}}}); adoptErr != nil {
		var unavailable AdoptUnavailableError
		if errors.As(adoptErr, &unavailable) {
			t.Fatalf("DriftAdopt still unavailable after Keep-upgrade: %v", adoptErr)
		}
		t.Fatalf("Apply(DriftAdopt): %v", adoptErr)
	}
}

// TestPlan_CorruptStateSourceRel_ReturnsTypedError pins that a
// hand-crafted state.json whose SourceRel escapes the asset folder
// (via "..") is rejected at load time with StateCorruptError. Defense
// in depth: asset.ResolveRelative would catch the write, but the load
// gate stops the tampered entry before Adopt reaches for it. See task
// 0035 review issue #3.
func TestPlan_CorruptStateSourceRel_ReturnsTypedError(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
		"AGENTS.md": {Hash: "previous", AssetID: "base", SourceRel: "../../etc/passwd"},
	}, nil)

	_, err := Plan(loaded, proj)
	if err == nil {
		t.Fatalf("expected StateCorruptError, got nil")
	}
	var corrupt StateCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("expected StateCorruptError, got %T: %v", err, err)
	}
}

// TestPlan_CorruptStateHalfPopulatedV3_ReturnsTypedError covers the
// "one of AssetID/SourceRel set, not both" wiring bug. A v2 legacy
// entry carries neither; a v3 entry carries both; anything in between
// is corruption.
func TestPlan_CorruptStateHalfPopulatedV3_ReturnsTypedError(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
		"AGENTS.md": {Hash: "previous", AssetID: "base"},
	}, nil)

	_, err := Plan(loaded, proj)
	var corrupt StateCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("expected StateCorruptError, got %T: %v", err, err)
	}
}

// TestPlan_CorruptStateWhitespaceOnlyAssetID_ReturnsTypedError covers
// the whitespace-only-slip case: `" "` for AssetID must be rejected
// like `""`.
func TestPlan_CorruptStateWhitespaceOnlyAssetID_ReturnsTypedError(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
		"AGENTS.md": {Hash: "previous", AssetID: " ", SourceRel: "AGENTS.md"},
	}, nil)

	_, err := Plan(loaded, proj)
	var corrupt StateCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("expected StateCorruptError, got %T: %v", err, err)
	}
}

// TestPlan_DriftAdoptEligibility_TrueOnV3StateEntry pins the drift
// row's AdoptProvenance: a state entry with both AssetID and SourceRel
// populated is v3 provenance, so Available() reports Adopt is legal.
func TestPlan_DriftAdoptEligibility_TrueOnV3StateEntry(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
		"AGENTS.md": {Hash: "previous", AssetID: "base", SourceRel: "AGENTS.md"},
	}, nil)

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}
	ch := findChange(t, preview.Changes, "AGENTS.md")
	if ch.Kind != ChangeDrift {
		t.Fatalf("kind = %q, want drift", ch.Kind)
	}
	if !ch.AdoptProvenance.Available() {
		t.Errorf("AdoptProvenance.Available() = false, want true for v3 state entry")
	}
	if ch.AdoptProvenance.AssetID != "base" || ch.AdoptProvenance.SourceRel != "AGENTS.md" {
		t.Errorf("AdoptProvenance = %+v, want {base AGENTS.md}", ch.AdoptProvenance)
	}
}

// TestPlan_DriftAdoptEligibility_FalseOnV2LegacyEntry pins the legacy
// v2 path: entries loaded via UnmarshalJSON's bare-hash branch carry
// empty AssetID/SourceRel, so Adopt is not available and the TUI must
// render the drift row as bilean.
func TestPlan_DriftAdoptEligibility_FalseOnV2LegacyEntry(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupProfileAndProject(t, projectRoot)
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStateV2Legacy(t, projectRoot, map[string]string{"AGENTS.md": "previous"}, nil)

	preview, err := Plan(loaded, proj)
	if err != nil {
		t.Fatal(err)
	}
	ch := findChange(t, preview.Changes, "AGENTS.md")
	if ch.Kind != ChangeDrift {
		t.Fatalf("kind = %q, want drift", ch.Kind)
	}
	if ch.AdoptProvenance.Available() {
		t.Errorf("AdoptProvenance.Available() = true, want false for legacy v2 entry")
	}
}

// TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds pins that
// AdoptProvenance is drift-only: create / update / unknown / delete rows
// all carry the zero value, so Available() is false. Each ChangeKind is
// its own subtest with an isolated fixture so a single-branch failure
// names itself instead of leaking into the others.
func TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds(t *testing.T) {
	// assertZeroProvenance fails when a row carries any adopt provenance.
	assertZeroProvenance := func(t *testing.T, ch FileChange, kind ChangeKind) {
		t.Helper()
		if ch.Kind != kind {
			t.Fatalf("kind = %q, want %q", ch.Kind, kind)
		}
		if ch.AdoptProvenance != (AdoptProvenance{}) {
			t.Errorf("%s AdoptProvenance = %+v, want zero", kind, ch.AdoptProvenance)
		}
	}

	// nonDriftFixture builds a project that produces one ChangeUpdate
	// (rendered body changed under the same baseline hash), one
	// ChangeUnknown (stray file inside a managed surface), and one
	// ChangeDelete (state-recorded file missing from desired), then
	// returns the plan. Each subtest reads its own row off the same
	// isolated fixture.
	nonDriftFixture := func(t *testing.T) *Preview {
		t.Helper()
		projectRoot := t.TempDir()
		loaded, proj := setupProfileAndProject(t, projectRoot)
		// On-disk AGENTS.md matches the prior baseline hash, so the file
		// is neither drift (hash matches baseline) nor create (file
		// exists) — the classifier returns ChangeUpdate.
		if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("baseline"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "stray.txt"), []byte("stray"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeStateEntries(t, projectRoot, map[string]ManagedFileEntry{
			"AGENTS.md":      {Hash: hashOf("baseline"), AssetID: "base", SourceRel: "AGENTS.md"},
			"gone-from-plan": {Hash: "stale", AssetID: "base", SourceRel: "gone"},
		}, nil)
		preview, err := Plan(loaded, proj)
		if err != nil {
			t.Fatal(err)
		}
		return preview
	}

	t.Run("ChangeCreate", func(t *testing.T) {
		projectRoot := t.TempDir()
		loaded, proj := setupProfileAndProject(t, projectRoot)
		preview, err := Plan(loaded, proj)
		if err != nil {
			t.Fatal(err)
		}
		assertZeroProvenance(t, findChange(t, preview.Changes, "AGENTS.md"), ChangeCreate)
	})

	t.Run("ChangeUpdate", func(t *testing.T) {
		preview := nonDriftFixture(t)
		assertZeroProvenance(t, findChange(t, preview.Changes, "AGENTS.md"), ChangeUpdate)
	})

	t.Run("ChangeUnknown", func(t *testing.T) {
		preview := nonDriftFixture(t)
		assertZeroProvenance(t, findChange(t, preview.Changes, ".codex/stray.txt"), ChangeUnknown)
	})

	t.Run("ChangeDelete", func(t *testing.T) {
		preview := nonDriftFixture(t)
		assertZeroProvenance(t, findChange(t, preview.Changes, "gone-from-plan"), ChangeDelete)
	})
}
