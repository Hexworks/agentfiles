package actions_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
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

	manifestPath := filepath.Join(f.ProfilePath, config.ProjectsDirName, "repo.json")
	if _, statErr := os.Stat(manifestPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected manifest removed, stat err = %v", statErr)
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

	preview, err := f.A.SyncProject(actions.SyncProjectInput{
		ProfileRef: "personal",
		ProjectID:  "repo",
	})
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if preview == nil {
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
		Drift: []app.DriftResolution{
			{Path: "AGENTS.md", Decision: app.DriftKeep},
		},
		Unknown: []app.UnknownResolution{
			{Path: ".codex/unrelated.md", Decision: app.UnknownKeep},
		},
	})
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	if _, planErr := f.A.PlanProject(actions.PlanProjectInput{ProfileRef: "personal", ProjectID: "repo"}); planErr != nil {
		t.Fatalf("re-plan: %v", planErr)
	}
}
