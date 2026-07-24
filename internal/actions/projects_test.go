package actions_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
)

func TestActions_AddProject_UnpacksInputs(t *testing.T) {
	f := newFixture(t, withProfile())
	repo := filepath.Join(f.Root, "repo")

	manifest, err := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          repo,
		EnabledAgents: []string{"codex"},
	})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if manifest.Name != "Repo" {
		t.Fatalf("expected name Repo, got %q", manifest.Name)
	}
	if manifest.Path != repo {
		t.Fatalf("expected path %q, got %q", repo, manifest.Path)
	}
}

func TestActions_AddProject_CollapsesValidationErrors(t *testing.T) {
	f := newFixture(t, withProfile())

	_, err := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(f.Root, "repo"),
		EnabledAgents: []string{"codex"},
		AssetIDs:      []string{"ghost-asset"},
	})
	if err == nil {
		t.Fatal("expected error for unknown asset id")
	}
	var aggregated errs.Errors
	if !errors.As(err, &aggregated) {
		t.Fatalf("expected errs.Errors, got %T: %v", err, err)
	}
	var typed app.AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError inside errs.Errors, got %v", err)
	}
}

func TestActions_LoadProject_ReturnsManifest(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErr := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(f.Root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	p, err := f.A.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if p.Name != "Repo" {
		t.Fatalf("expected name Repo, got %q", p.Name)
	}
}

func TestActions_LoadProject_MissingReturnsProjectNotFoundError(t *testing.T) {
	f := newFixture(t, withProfile())

	_, err := f.A.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed app.ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_UpdateProject_PersistsChanges(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErr := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(f.Root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}
	p, loadErr := f.A.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if loadErr != nil {
		t.Fatalf("load: %v", loadErr)
	}

	if _, err := f.A.UpdateProject(actions.UpdateProjectInput{
		ProfileRef:    "personal",
		ProjectID:     p.ID,
		Name:          p.Name,
		Path:          p.Path,
		EnabledAgents: []string{"codex", "claude-code"},
	}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	reloaded, _ := f.A.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if len(reloaded.EnabledAgents) != 2 {
		t.Fatalf("expected 2 agents persisted, got %v", reloaded.EnabledAgents)
	}
}

func TestActions_DeleteProject_RemovesManifestOnly(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErr := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(f.Root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	if _, err := f.A.DeleteProject(actions.DeleteProjectInput{ProfileRef: "personal", ProjectID: "repo"}); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	stored, listErr := f.Svc.Projects.ListByProfile("personal")
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	if len(stored) != 0 {
		t.Fatalf("expected empty projects group, got %d", len(stored))
	}
}

func TestActions_PlanProject_ReturnsPreview(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErr := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(f.Root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	preview, err := f.A.PlanProject(actions.PlanProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if err != nil {
		t.Fatalf("PlanProject: %v", err)
	}
	if preview == nil {
		t.Fatal("expected non-nil preview")
	}
}

// TestActions_DiffFile_ForwardsToService is a light forwarding assertion: the
// input struct's ids reach Service.DiffFile, which returns ProjectNotFoundError
// for an unknown project. A missing-project forward is enough to prove the
// seam plumbs ProfileRef/ProjectID/Path through without a heavy render fixture.
func TestActions_DiffFile_ForwardsToService(t *testing.T) {
	f := newFixture(t, withProfile())

	_, err := f.A.DiffFile(actions.DiffFileInput{
		ProfileRef: "personal",
		ProjectID:  "ghost",
		Path:       ".claude/skills/x/SKILL.md",
	})
	if err == nil {
		t.Fatal("expected error for unknown project")
	}
	var typed app.ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("err = %T (%v), want ProjectNotFoundError", err, err)
	}
}

func TestActions_SyncProject_AppliesAndReturnsPreview(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErr := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(f.Root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	out, err := f.A.SyncProject(actions.SyncProjectInput{
		ProfileRef: "personal",
		ProjectID:  "repo",
	})
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if out.Preview == nil {
		t.Fatal("expected non-nil preview")
	}
}

func TestActions_SyncProject_PassesDriftAndUnknownResolutions(t *testing.T) {
	f := newFixture(t, withProfile())
	if _, addErr := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(f.Root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	// Resolutions for paths that don't exist as drift/unknown in this
	// preview — Service should ignore them silently. The point of this
	// test is to confirm the action surface forwards the slices without
	// dropping them.
	_, err := f.A.SyncProject(actions.SyncProjectInput{
		ProfileRef: "personal",
		ProjectID:  "repo",
		Drift: []appapi.DriftResolution{
			{Path: "AGENTS.md", Decision: appapi.DriftKeep},
		},
		Unknown: []appapi.UnknownResolution{
			{Path: ".codex/unrelated.md", Decision: appapi.UnknownKeep},
		},
	})
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	if _, planErr := f.A.PlanProject(actions.PlanProjectInput{ProfileRef: "personal", ProjectID: "repo"}); planErr != nil {
		t.Fatalf("re-plan: %v", planErr)
	}
}

func TestActions_SyncProject_PersistsIgnoredPaths(t *testing.T) {
	f := newFixture(t, withProfile())
	repo := filepath.Join(f.Root, "repo")
	if _, addErr := f.A.AddProject(actions.AddProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          repo,
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	// First sync writes state.json so the next plan runs unknown detection
	// (skipped on first apply). Only then is an all-unknown folder eligible
	// to be ignored — Service.Apply re-asserts that eligibility before
	// persisting, so the path must actually exist as an unknown folder.
	if _, err := f.A.SyncProject(actions.SyncProjectInput{ProfileRef: "personal", ProjectID: "repo"}); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	legacy := filepath.Join(repo, ".codex", "skills", "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := f.A.SyncProject(actions.SyncProjectInput{
		ProfileRef:   "personal",
		ProjectID:    "repo",
		IgnoredPaths: []string{".codex/skills/legacy"},
	}); err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(repo, config.StateDirName, config.StateFileName))
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	if !strings.Contains(string(data), `".codex/skills/legacy"`) {
		t.Fatalf("state.json missing ignored path, got: %s", data)
	}
}
