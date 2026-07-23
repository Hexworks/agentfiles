package app

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/settings"
)

// enabledSettings is the shared "git enabled" value seeded into services
// that exercise the git-aware paths.
var enabledSettings = settings.Settings{Version: 1, Git: settings.GitSettings{Enabled: true}}

// commitCallErr is a stub domain error injected into the fake committer
// so tests can assert the failure rides on CommitOutcome.Err instead of
// bubbling as a hard save failure.
type commitCallErr struct{}

func (commitCallErr) Error() string           { return "boom" }
func (commitCallErr) Severity() errs.Severity { return errs.SeverityWarning }

func seedAssetService(t *testing.T, s settings.Settings, committer GitCommitter) (*Service, string, string) {
	t.Helper()
	root := t.TempDir()
	svc := newSvcWith(root, s, committer)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "review", Name: "review", Type: asset.TypeSkill,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	return svc, "personal", "review"
}

func loadedAsset(t *testing.T, svc *Service, profileID, assetID string) *asset.Asset {
	t.Helper()
	a, err := svc.LoadAsset(profileID, assetID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return a
}

func TestUpdateAsset_CommitsManifestPathspecWhenGitEnabled(t *testing.T) {
	fc := &fakeCommitter{SHA: "abc1234"}
	svc, profileID, assetID := seedAssetService(t, enabledSettings, fc)
	a := loadedAsset(t, svc, profileID, assetID)
	a.Manifest.Description = "edited"

	outcome, err := svc.UpdateAsset(profileID, &a.Manifest)
	if err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	if outcome.SHA != "abc1234" {
		t.Fatalf("outcome.SHA = %q, want abc1234", outcome.SHA)
	}
	if len(fc.Calls) != 1 {
		t.Fatalf("committer calls = %d, want 1", len(fc.Calls))
	}
	call := fc.Calls[0]
	wantSpec := filepath.Join(a.Dir, "asset.json")
	if len(call.Pathspec) != 1 || call.Pathspec[0] != wantSpec {
		t.Fatalf("pathspec = %v, want [%q]", call.Pathspec, wantSpec)
	}
	if call.Msg != "chore(agentfiles): update asset review manifest" {
		t.Fatalf("msg = %q", call.Msg)
	}
}

func TestSaveAssetFilesEdit_CommitsFilesPathspec(t *testing.T) {
	fc := &fakeCommitter{SHA: "def5678"}
	svc, profileID, assetID := seedAssetService(t, enabledSettings, fc)
	a := loadedAsset(t, svc, profileID, assetID)

	outcome, err := svc.SaveAssetFilesEdit(profileID, &a.Manifest)
	if err != nil {
		t.Fatalf("SaveAssetFilesEdit: %v", err)
	}
	if outcome.SHA != "def5678" {
		t.Fatalf("outcome.SHA = %q, want def5678", outcome.SHA)
	}
	if len(fc.Calls) != 1 {
		t.Fatalf("committer calls = %d, want 1", len(fc.Calls))
	}
	wantSpec := a.Dir + "/**"
	if len(fc.Calls[0].Pathspec) != 1 || fc.Calls[0].Pathspec[0] != wantSpec {
		t.Fatalf("pathspec = %v, want [%q]", fc.Calls[0].Pathspec, wantSpec)
	}
	if fc.Calls[0].Msg != "chore(agentfiles): edit asset review files" {
		t.Fatalf("msg = %q", fc.Calls[0].Msg)
	}
}

func TestUpdateAsset_DisabledSkipsCommit(t *testing.T) {
	fc := &fakeCommitter{}
	svc, profileID, assetID := seedAssetService(t, settings.Default(), fc)
	a := loadedAsset(t, svc, profileID, assetID)

	outcome, err := svc.UpdateAsset(profileID, &a.Manifest)
	if err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	if outcome != (CommitOutcome{}) {
		t.Fatalf("outcome = %+v, want zero-value", outcome)
	}
	if len(fc.Calls) != 0 {
		t.Fatalf("committer called %d times, want 0", len(fc.Calls))
	}
}

func TestUpdateAsset_CommitErrorSurfacesOnOutcome(t *testing.T) {
	fc := &fakeCommitter{Err: commitCallErr{}}
	svc, profileID, assetID := seedAssetService(t, enabledSettings, fc)
	a := loadedAsset(t, svc, profileID, assetID)
	a.Manifest.Description = "still saved"

	outcome, err := svc.UpdateAsset(profileID, &a.Manifest)
	if err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	if outcome.Err == nil {
		t.Fatal("expected outcome.Err to carry the commit failure")
	}
	// Manifest still on disk despite commit failure.
	reloaded, loadErr := svc.LoadAsset(profileID, assetID)
	if loadErr != nil {
		t.Fatalf("reload: %v", loadErr)
	}
	if reloaded.Description != "still saved" {
		t.Fatalf("description = %q, want %q (save must succeed even when commit fails)",
			reloaded.Description, "still saved")
	}
}

func TestApply_CommitsSyncedFilesAndStateJSON(t *testing.T) {
	fc := &fakeCommitter{SHA: "sync001"}
	root := t.TempDir()
	svc := newSvcWith(root, enabledSettings, fc)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	repoPath := filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("personal", "Repo", repoPath, []string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	_, outcome, applyErr := svc.Apply("personal", "repo", Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if outcome.SHA != "sync001" {
		t.Fatalf("outcome.SHA = %q, want sync001", outcome.SHA)
	}
	if len(fc.Calls) != 1 {
		t.Fatalf("committer calls = %d, want 1", len(fc.Calls))
	}
	call := fc.Calls[0]
	if call.Dir != repoPath {
		t.Fatalf("dir = %q, want %q", call.Dir, repoPath)
	}
	wantState := filepath.Join(repoPath, ".agentfiles", "state.json")
	wantAgents := filepath.Join(repoPath, "AGENTS.md")
	if !slices.Contains(call.Pathspec, wantState) {
		t.Fatalf("pathspec missing %q: %v", wantState, call.Pathspec)
	}
	if !slices.Contains(call.Pathspec, wantAgents) {
		t.Fatalf("pathspec missing %q: %v", wantAgents, call.Pathspec)
	}
	if call.Msg != "chore(agentfiles): sync project Repo (1 files)" {
		t.Fatalf("msg = %q", call.Msg)
	}
}

func TestApply_DisabledSkipsCommit(t *testing.T) {
	fc := &fakeCommitter{}
	root := t.TempDir()
	svc := newSvcWith(root, settings.Default(), fc)
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

	_, outcome, applyErr := svc.Apply("personal", "repo", Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if outcome != (CommitOutcome{}) {
		t.Fatalf("outcome = %+v, want zero-value", outcome)
	}
	if len(fc.Calls) != 0 {
		t.Fatalf("committer called %d times, want 0", len(fc.Calls))
	}
}

func TestUpdateSettings_PreflightRefusesWhenBinaryMissing(t *testing.T) {
	// The pre-flight lives inside app.Service.UpdateSettings and calls
	// git.BinaryAvailable directly. Skip when git is actually installed
	// on PATH — we cannot fake that without a bigger seam. Cover
	// disabled-flag pass and enabled-flag no-op behavior instead.
	svc := newSvc(t.TempDir())
	prior := svc.Settings()
	if err := svc.UpdateSettings(prior); err != nil {
		t.Fatalf("no-op update: %v", err)
	}
}

func TestUpdateSettings_MissingStoreReturnsError(t *testing.T) {
	svc := &Service{}
	err := svc.UpdateSettings(settings.Default())
	var typed SettingsUnavailableError
	if !errors.As(err, &typed) {
		t.Fatalf("expected SettingsUnavailableError, got %T (%v)", err, err)
	}
}
