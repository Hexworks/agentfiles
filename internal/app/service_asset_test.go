package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
)

func TestLoadAsset_ReturnsExisting(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}

	a, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if a.ID != "agents" {
		t.Fatalf("expected id agents, got %q", a.ID)
	}
}

func TestLoadAsset_MissingReturnsAssetNotFoundError(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	_, err := svc.LoadAsset("personal", "missing")

	var typed AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
	if typed.AssetID != "missing" {
		t.Fatalf("expected id preserved, got %q", typed.AssetID)
	}
}

func TestUpdateAsset_OverwritesManifestOnDisk(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	a, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	a.Description = "updated"
	a.Tags = []string{"review"}
	if err := svc.UpdateAsset("personal", &a.Manifest); err != nil {
		t.Fatalf("update: %v", err)
	}

	reloaded, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Description != "updated" {
		t.Fatalf("expected description updated, got %q", reloaded.Description)
	}
	if len(reloaded.Tags) != 1 || reloaded.Tags[0] != "review" {
		t.Fatalf("expected tags [review], got %v", reloaded.Tags)
	}
}

func TestUpdateAsset_MissingAssetReturnsError(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	stranger := &asset.Manifest{ID: "stranger", Name: "stranger", Type: asset.TypeAgentsDoc}
	err := svc.UpdateAsset("personal", stranger)

	var typed AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

func TestUpdateAsset_IgnoresCallerSuppliedDir(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}

	a, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	authoritativeDir := a.Dir

	// Caller mutates Description, then the manifest is written via the
	// trusted dir resolved from the loaded profile — the caller cannot
	// redirect the write because UpdateAsset takes only the manifest.
	edit := a.Manifest
	edit.Description = "edited"
	if err := svc.UpdateAsset("personal", &edit); err != nil {
		t.Fatalf("update: %v", err)
	}

	reloaded, err := svc.LoadAsset("personal", "agents")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Dir != authoritativeDir {
		t.Fatalf("expected dir unchanged %q, got %q", authoritativeDir, reloaded.Dir)
	}
	if reloaded.Description != "edited" {
		t.Fatalf("expected description edited, got %q", reloaded.Description)
	}
}

func TestDeleteAsset_RemovesAssetDirectory(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	dir, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	})
	if err != nil {
		t.Fatalf("init asset: %v", err)
	}

	if err := svc.DeleteAsset("personal", "agents"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("expected asset dir removed, stat err = %v", statErr)
	}
}

func TestDeleteAsset_UnselectsAssetFromProjects(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	if err := svc.DeleteAsset("personal", "agents"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	p, err := svc.LoadProject("personal", "repo")
	if err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if len(p.SelectedAssetIDs) != 0 {
		t.Fatalf("expected SelectedAssetIDs cleared, got %v", p.SelectedAssetIDs)
	}
}

func TestDeleteAsset_UnselectsAcrossMultipleProjects(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		if _, addErrs := svc.AddProject("personal", name, filepath.Join(root, name),
			[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
			t.Fatalf("add project %s: %v", name, addErrs)
		}
	}

	if err := svc.DeleteAsset("personal", "agents"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	for _, id := range []string{"alpha", "beta", "gamma"} {
		p, err := svc.LoadProject("personal", id)
		if err != nil {
			t.Fatalf("reload %s: %v", id, err)
		}
		if len(p.SelectedAssetIDs) != 0 {
			t.Fatalf("project %s expected cleared SelectedAssetIDs, got %v", id, p.SelectedAssetIDs)
		}
	}
}

func TestService_SelectAsset_AddsToProject(t *testing.T) {
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")

	got, err := svc.SelectAsset(profileID, projectID, "agents")
	if err != nil {
		t.Fatalf("SelectAsset: %v", err)
	}
	if len(got) != 1 || got[0] != "agents" {
		t.Fatalf("returned selection = %v, want [agents]", got)
	}
	p, err := svc.LoadProject(profileID, projectID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(p.SelectedAssetIDs) != 1 || p.SelectedAssetIDs[0] != "agents" {
		t.Fatalf("persisted selection = %v, want [agents]", p.SelectedAssetIDs)
	}
}

func TestService_SelectAsset_IdempotentOnDuplicate(t *testing.T) {
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")
	if _, err := svc.SelectAsset(profileID, projectID, "agents"); err != nil {
		t.Fatalf("seed first call: %v", err)
	}

	got, err := svc.SelectAsset(profileID, projectID, "agents")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(got) != 1 || got[0] != "agents" {
		t.Fatalf("returned selection = %v, want [agents] (no duplicate)", got)
	}
	p, _ := svc.LoadProject(profileID, projectID)
	if len(p.SelectedAssetIDs) != 1 {
		t.Fatalf("expected single occurrence, got %v", p.SelectedAssetIDs)
	}
}

func TestService_SelectAsset_MissingAssetReturnsAssetNotFoundError(t *testing.T) {
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")

	_, err := svc.SelectAsset(profileID, projectID, "missing")
	var typed AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

func TestService_SelectAsset_MissingProjectReturnsProjectNotFoundError(t *testing.T) {
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}

	_, err := svc.SelectAsset("personal", "missing-project", "agents")
	var typed ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
}

func TestService_UnselectAsset_RemovesFromProject(t *testing.T) {
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")
	if _, err := svc.SelectAsset(profileID, projectID, "agents"); err != nil {
		t.Fatalf("seed select: %v", err)
	}

	got, err := svc.UnselectAsset(profileID, projectID, "agents")
	if err != nil {
		t.Fatalf("UnselectAsset: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("returned selection = %v, want []", got)
	}
	p, _ := svc.LoadProject(profileID, projectID)
	if len(p.SelectedAssetIDs) != 0 {
		t.Fatalf("persisted selection = %v, want []", p.SelectedAssetIDs)
	}
}

func TestService_UnselectAsset_IdempotentOnNeverSelected(t *testing.T) {
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")

	got, err := svc.UnselectAsset(profileID, projectID, "agents")
	if err != nil {
		t.Fatalf("UnselectAsset on never-selected asset: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("returned selection = %v, want []", got)
	}
}

func TestService_UnselectAsset_MissingAssetReturnsAssetNotFoundError(t *testing.T) {
	svc, profileID, projectID := seedServiceWithProjectAndAsset(t, "agents")

	_, err := svc.UnselectAsset(profileID, projectID, "missing")
	var typed AssetNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetNotFoundError, got %T: %v", err, err)
	}
}

// seedServiceWithProjectAndAsset creates a temp-rooted service with one
// profile, one asset, and one project so the Select/Unselect tests can
// share the boilerplate.
func seedServiceWithProjectAndAsset(t *testing.T, assetID string) (svc *Service, profileID, projectID string) {
	t.Helper()
	root := t.TempDir()
	svc = New(filepath.Join(root, "registry.json"))
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	profileID = "personal"
	if _, err := svc.InitAsset(profileID, asset.Manifest{
		ID: assetID, Name: assetID, Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	if _, addErrs := svc.AddProject(profileID, "Repo", filepath.Join(root, "repo"),
		[]string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}
	projectID = "repo"
	return svc, profileID, projectID
}

func TestDeleteAsset_PartialFailureLeavesRecoverableState(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based failure injection cannot run as root")
	}
	root := t.TempDir()
	svc := New(filepath.Join(root, "registry.json"))
	profilePath := filepath.Join(root, "profile")
	if _, err := svc.CreateProfile("Personal", profilePath); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", filepath.Join(root, "repo"),
		[]string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	// Make the projects store file unwritable so the per-project save
	// fails mid-loop. The asset folder removal in DeleteAsset is separate
	// and still runs, so the second half of the operation should succeed
	// even though the first half fails.
	storePath := svc.Projects.Path
	if err := os.Chmod(storePath, 0o400); err != nil {
		t.Fatalf("chmod store file: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(storePath, 0o644) })

	err := svc.DeleteAsset("personal", "agents")
	if err == nil {
		t.Fatal("expected aggregated failure error")
	}
	leaves := errs.Collect(err)
	if len(leaves) == 0 {
		t.Fatal("expected at least one typed leaf")
	}

	// Restore write permission so the recovery run can finish.
	if err := os.Chmod(storePath, 0o644); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}
	// Re-running DeleteAsset converges: by now the asset folder is gone,
	// so the call should fail with AssetNotFoundError. The project save
	// can proceed manually via UpdateProject if needed, but the key
	// invariant is that the system is recoverable.
	recoveryErr := svc.DeleteAsset("personal", "agents")
	var notFound AssetNotFoundError
	if !errors.As(recoveryErr, &notFound) {
		t.Fatalf("expected AssetNotFoundError on recovery, got %T: %v", recoveryErr, recoveryErr)
	}
}
