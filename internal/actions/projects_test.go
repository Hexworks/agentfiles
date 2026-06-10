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

// newProjectFixture returns a fixture with a profile already created.
// The profile id is "personal" (slug of "Personal"). The repo path is
// the caller's responsibility; ProfilePath is returned for callers
// that need to inspect the manifest dir directly.
func newProjectFixture(t *testing.T) (a *actions.Actions, svc *app.Service, profilePath, root string) {
	t.Helper()
	root = t.TempDir()
	svc = app.New(filepath.Join(root, "registry.json"))
	profilePath = filepath.Join(root, "profile")
	if _, err := svc.CreateProfile("Personal", profilePath); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	a = actions.New(svc)
	return a, svc, profilePath, root
}

func TestActions_RegisterProject_UnpacksInputs(t *testing.T) {
	a, _, _, root := newProjectFixture(t)
	repo := filepath.Join(root, "repo")

	manifest, err := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          repo,
		EnabledAgents: []string{"codex"},
		AssetIDs:      nil,
	})
	if err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}
	if manifest.Name != "Repo" {
		t.Fatalf("expected name Repo, got %q", manifest.Name)
	}
	if manifest.Path != repo {
		t.Fatalf("expected path %q, got %q", repo, manifest.Path)
	}
}

func TestActions_RegisterProject_CollapsesValidationErrors(t *testing.T) {
	a, _, _, root := newProjectFixture(t)

	_, err := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
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
	a, _, _, root := newProjectFixture(t)
	if _, addErr := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	p, err := a.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if p.Name != "Repo" {
		t.Fatalf("expected name Repo, got %q", p.Name)
	}
}

func TestActions_LoadProject_MissingReturnsProjectNotFoundError(t *testing.T) {
	a, _, _, _ := newProjectFixture(t)

	_, err := a.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed app.ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
}

func TestActions_UpdateProject_PersistsChanges(t *testing.T) {
	a, _, _, root := newProjectFixture(t)
	if _, addErr := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}
	p, loadErr := a.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if loadErr != nil {
		t.Fatalf("load: %v", loadErr)
	}
	p.EnabledAgents = []string{"codex", "claude-code"}

	if _, err := a.UpdateProject(actions.UpdateProjectInput{ProfileRef: "personal", Project: p}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	reloaded, _ := a.LoadProject(actions.LoadProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if len(reloaded.EnabledAgents) != 2 {
		t.Fatalf("expected 2 agents persisted, got %v", reloaded.EnabledAgents)
	}
}

func TestActions_DeleteProject_RemovesManifestOnly(t *testing.T) {
	a, _, profilePath, root := newProjectFixture(t)
	if _, addErr := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	if _, err := a.DeleteProject(actions.DeleteProjectInput{ProfileRef: "personal", ProjectID: "repo"}); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	manifestPath := filepath.Join(profilePath, config.ProjectsDirName, "repo.json")
	if _, statErr := os.Stat(manifestPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected manifest removed, stat err = %v", statErr)
	}
}

func TestActions_PlanProject_ReturnsPreview(t *testing.T) {
	a, _, _, root := newProjectFixture(t)
	if _, addErr := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	preview, err := a.PlanProject(actions.PlanProjectInput{ProfileRef: "personal", ProjectID: "repo"})
	if err != nil {
		t.Fatalf("PlanProject: %v", err)
	}
	if preview == nil {
		t.Fatal("expected non-nil preview")
	}
}

func TestActions_SyncProject_AppliesAndReturnsPreview(t *testing.T) {
	a, _, _, root := newProjectFixture(t)
	if _, addErr := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	preview, err := a.SyncProject(actions.SyncProjectInput{
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
	a, _, _, root := newProjectFixture(t)
	if _, addErr := a.RegisterProject(actions.RegisterProjectInput{
		ProfileRef:    "personal",
		Name:          "Repo",
		Path:          filepath.Join(root, "repo"),
		EnabledAgents: []string{"codex"},
	}); addErr != nil {
		t.Fatalf("seed: %v", addErr)
	}

	// Resolutions for paths that don't exist as drift/unknown in this
	// preview — Service should ignore them silently. The point of this
	// test is to confirm the action surface forwards the slices without
	// dropping them.
	_, err := a.SyncProject(actions.SyncProjectInput{
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

	// Sanity: preview-shape check by re-planning afterwards.
	if _, planErr := a.PlanProject(actions.PlanProjectInput{ProfileRef: "personal", ProjectID: "repo"}); planErr != nil {
		t.Fatalf("re-plan: %v", planErr)
	}
}
