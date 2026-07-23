package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/git"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/settings"
)

// enabledSettings is the shared "git enabled" value seeded into services
// that exercise the git-aware paths.
var enabledSettings = settings.Settings{Version: 1, Git: settings.GitSettings{Enabled: true}}

// commitCallErr is a stub domain error injected into the fake committer
// so tests can assert the failure rides on appapi.Failed instead of
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

func mustCommitted(t *testing.T, outcome appapi.CommitOutcome) appapi.Committed {
	t.Helper()
	c, ok := outcome.(appapi.Committed)
	if !ok {
		t.Fatalf("expected appapi.Committed, got %T (%+v)", outcome, outcome)
	}
	return c
}

func mustSkipped(t *testing.T, outcome appapi.CommitOutcome, want appapi.SkipReason) {
	t.Helper()
	s, ok := outcome.(appapi.Skipped)
	if !ok {
		t.Fatalf("expected appapi.Skipped{%v}, got %T (%+v)", want, outcome, outcome)
	}
	if s.Reason != want {
		t.Fatalf("skip reason = %v, want %v", s.Reason, want)
	}
}

func mustFailed(t *testing.T, outcome appapi.CommitOutcome) appapi.Failed {
	t.Helper()
	f, ok := outcome.(appapi.Failed)
	if !ok {
		t.Fatalf("expected appapi.Failed, got %T (%+v)", outcome, outcome)
	}
	return f
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
	if got := mustCommitted(t, outcome); got.SHA != "abc1234" {
		t.Fatalf("SHA = %q, want abc1234", got.SHA)
	}
	if len(fc.Calls) != 1 {
		t.Fatalf("committer calls = %d, want 1", len(fc.Calls))
	}
	call := fc.Calls[0]
	// Ground-truth pathspec suffix, not the config constant — a rename
	// of AssetManifestFileName must break this assertion, not sail
	// through in lockstep. See docs/guidelines/testing.md.
	if len(call.Pathspec) != 1 || !strings.HasSuffix(call.Pathspec[0], "/asset.json") {
		t.Fatalf("pathspec = %v, want single entry ending in /asset.json", call.Pathspec)
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
	if got := mustCommitted(t, outcome); got.SHA != "def5678" {
		t.Fatalf("SHA = %q, want def5678", got.SHA)
	}
	if len(fc.Calls) != 1 {
		t.Fatalf("committer calls = %d, want 1", len(fc.Calls))
	}
	if len(fc.Calls[0].Pathspec) != 1 || !strings.HasSuffix(fc.Calls[0].Pathspec[0], "/**") {
		t.Fatalf("pathspec = %v, want single entry ending in /**", fc.Calls[0].Pathspec)
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
	mustSkipped(t, outcome, appapi.SkipDisabled)
	if len(fc.Calls) != 0 {
		t.Fatalf("committer called %d times, want 0", len(fc.Calls))
	}
}

func TestUpdateAsset_CommitFailureSurfacesOnOutcome(t *testing.T) {
	fc := &fakeCommitter{Outcome: appapi.Failed{Err: commitCallErr{}}}
	svc, profileID, assetID := seedAssetService(t, enabledSettings, fc)
	a := loadedAsset(t, svc, profileID, assetID)
	a.Manifest.Description = "still saved"

	outcome, err := svc.UpdateAsset(profileID, &a.Manifest)
	if err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	failed := mustFailed(t, outcome)
	if failed.Err == nil {
		t.Fatal("expected appapi.Failed.Err to carry the commit failure")
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

	_, outcome, applyErr := svc.Apply("personal", "repo", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if got := mustCommitted(t, outcome); got.SHA != "sync001" {
		t.Fatalf("SHA = %q, want sync001", got.SHA)
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

	_, outcome, applyErr := svc.Apply("personal", "repo", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	mustSkipped(t, outcome, appapi.SkipDisabled)
	if len(fc.Calls) != 0 {
		t.Fatalf("committer called %d times, want 0", len(fc.Calls))
	}
}

func TestUpdateSettings_NoOpUpdateSucceeds(t *testing.T) {
	// A no-op UpdateSettings never crosses the enable-transition, so the
	// pre-flight is not invoked. Confirms the disabled→disabled path
	// stays a silent success regardless of what the fake reports.
	svc := newSvc(t.TempDir())
	prior := svc.Settings()
	if err := svc.UpdateSettings(prior); err != nil {
		t.Fatalf("no-op update: %v", err)
	}
}

func TestUpdateSettings_PreflightRefusesWhenBinaryMissing(t *testing.T) {
	// The pre-flight fires only on the disabled→enabled transition. The
	// fake reports a BinaryMissingError so the save is refused without
	// touching $PATH.
	fc := &fakeCommitter{BinaryErr: stubBinaryMissing{}}
	root := t.TempDir()
	svc := newSvcWith(root, settings.Default(), fc)

	err := svc.UpdateSettings(settings.Settings{
		Version: settings.Version,
		Git:     settings.GitSettings{Enabled: true},
	})
	if err == nil {
		t.Fatal("expected pre-flight refusal, got nil")
	}
	var missing stubBinaryMissing
	if !errors.As(err, &missing) {
		t.Fatalf("expected stubBinaryMissing, got %T (%v)", err, err)
	}
	// Confirm the store was not touched — settings on disk are absent.
	if _, statErr := os.Stat(svc.SettingsPath()); !os.IsNotExist(statErr) {
		t.Fatalf("settings.json exists after refused save (stat err: %v)", statErr)
	}
	// And the cached value is unchanged.
	if svc.Settings().Git.Enabled {
		t.Fatalf("cached settings still show git enabled after refusal")
	}
}

// stubBinaryMissing is a test-local BinaryMissingError analogue so the
// fake committer can return one without importing internal/git into
// service.go.
type stubBinaryMissing struct{}

func (stubBinaryMissing) Error() string           { return "git binary not found on PATH" }
func (stubBinaryMissing) Severity() errs.Severity { return errs.SeverityWarning }

func TestUpdateSettings_MissingStoreReturnsError(t *testing.T) {
	svc := &Service{}
	err := svc.UpdateSettings(settings.Default())
	var typed SettingsUnavailableError
	if !errors.As(err, &typed) {
		t.Fatalf("expected SettingsUnavailableError, got %T (%v)", err, err)
	}
}

// -- Cross-boundary integration tests --
//
// The fake-committer tests above assert the (dir, pathspec, msg)
// tuple, but the pathspec value is composed by the same code the
// service composed it with. Bug 8e7b638 slipped past that assertion:
// the pathspec suffix looked right in a unit test but did not match
// the real on-disk layout. These tests wire the production
// NewGitCommitter, seed a real git repo with t.TempDir(), and assert
// against `git log --name-only HEAD` — ground truth, not a copy.
// docs/guidelines/testing.md#cross-boundary-integration cites this
// task by name.

func requireGitBinary(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git binary not available: %v", err)
	}
}

func initRealRepo(t *testing.T, dir string) {
	t.Helper()
	runShellGit(t, dir, "init", "-q")
	runShellGit(t, dir, "config", "user.email", "test@example.com")
	runShellGit(t, dir, "config", "user.name", "Test User")
	runShellGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "SEED.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runShellGit(t, dir, "add", "SEED.md")
	runShellGit(t, dir, "commit", "-q", "-m", "seed")
}

func runShellGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func realHeadFiles(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "show", "--name-only", "--format=", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show: %v\n%s", err, out)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files
}

func newRealSvc(t *testing.T, profileParent string) *Service {
	t.Helper()
	dataRoot := t.TempDir()
	return NewWithStores(
		registry.NewStore(filepath.Join(dataRoot, "registry.json")),
		projectstore.NewStore(filepath.Join(dataRoot, "projects.json")),
		settings.NewStore(filepath.Join(dataRoot, "settings.json")),
		enabledSettings,
		NewGitCommitter(),
	)
}

func TestUpdateAsset_RealGitRecordsManifestCommit(t *testing.T) {
	requireGitBinary(t)
	repoRoot := t.TempDir()
	initRealRepo(t, repoRoot)
	profileParent := filepath.Join(repoRoot, "profiles")
	if err := os.MkdirAll(profileParent, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := newRealSvc(t, profileParent)
	if _, err := svc.CreateProfile("Personal", filepath.Join(profileParent, "personal")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "review", Name: "review", Type: asset.TypeSkill,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	a := loadedAsset(t, svc, "personal", "review")
	a.Manifest.Description = "edited by real-git test"

	outcome, err := svc.UpdateAsset("personal", &a.Manifest)
	if err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	c := mustCommitted(t, outcome)
	if !isShortSHA(c.SHA) {
		t.Fatalf("SHA = %q, expected short hex", c.SHA)
	}
	files := realHeadFiles(t, repoRoot)
	wantSuffix := "assets/skill/review/asset.json"
	if len(files) != 1 || !strings.HasSuffix(files[0], wantSuffix) {
		t.Fatalf("HEAD files = %v, want single entry ending in %q", files, wantSuffix)
	}
}

func TestSaveAssetFilesEdit_RealGitRecordsFilesCommit(t *testing.T) {
	requireGitBinary(t)
	repoRoot := t.TempDir()
	initRealRepo(t, repoRoot)
	profileParent := filepath.Join(repoRoot, "profiles")
	if err := os.MkdirAll(profileParent, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := newRealSvc(t, profileParent)
	if _, err := svc.CreateProfile("Personal", filepath.Join(profileParent, "personal")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "review", Name: "review", Type: asset.TypeSkill,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	a := loadedAsset(t, svc, "personal", "review")
	// Simulate the external editor writing a new file inside the asset
	// dir. SaveAssetFilesEdit's `assets/<type>/<id>/**` pathspec must
	// cover it.
	extra := filepath.Join(a.Dir, "notes.md")
	if err := os.WriteFile(extra, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Ensure the manifest also changes so the commit is not empty when
	// the pathspec is somehow scoped too narrowly.
	a.Manifest.Description = "editor round-trip"

	outcome, err := svc.SaveAssetFilesEdit("personal", &a.Manifest)
	if err != nil {
		t.Fatalf("SaveAssetFilesEdit: %v", err)
	}
	if _, ok := outcome.(appapi.Committed); !ok {
		t.Fatalf("expected Committed, got %T (%+v)", outcome, outcome)
	}
	files := realHeadFiles(t, repoRoot)
	wantNotes := "notes.md"
	wantManifest := "asset.json"
	sawNotes := false
	sawManifest := false
	for _, f := range files {
		if strings.HasSuffix(f, "/"+wantNotes) {
			sawNotes = true
		}
		if strings.HasSuffix(f, "/"+wantManifest) {
			sawManifest = true
		}
	}
	if !sawNotes || !sawManifest {
		t.Fatalf("HEAD files = %v, want to see both %q and %q", files, wantNotes, wantManifest)
	}
}

func TestApply_RealGitRecordsProjectCommit(t *testing.T) {
	requireGitBinary(t)
	profileParent := t.TempDir()
	targetRepo := t.TempDir()
	initRealRepo(t, targetRepo)
	svc := newRealSvc(t, profileParent)
	if _, err := svc.CreateProfile("Personal", filepath.Join(profileParent, "personal")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Repo", targetRepo, []string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	_, outcome, applyErr := svc.Apply("personal", "repo", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if _, ok := outcome.(appapi.Committed); !ok {
		t.Fatalf("expected Committed, got %T (%+v)", outcome, outcome)
	}
	files := realHeadFiles(t, targetRepo)
	wantAgents := "AGENTS.md"
	wantState := ".agentfiles/state.json"
	sawAgents := false
	sawState := false
	for _, f := range files {
		if f == wantAgents {
			sawAgents = true
		}
		if f == wantState {
			sawState = true
		}
	}
	if !sawAgents || !sawState {
		t.Fatalf("HEAD files = %v, want both %q and %q", files, wantAgents, wantState)
	}
}

// TestApply_RealGitProfileNestedInsideOuterRepo covers the shape the
// shipped bug (8e7b638) sailed through: a profile folder sitting deep
// inside a larger repo. The commit must land on the nested state file
// even though its pathspec is expressed absolutely.
func TestApply_RealGitProfileNestedInsideOuterRepo(t *testing.T) {
	requireGitBinary(t)
	outer := t.TempDir()
	initRealRepo(t, outer)
	profileParent := filepath.Join(outer, "profiles")
	if err := os.MkdirAll(profileParent, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outer, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := newRealSvc(t, profileParent)
	if _, err := svc.CreateProfile("Personal", filepath.Join(profileParent, "personal")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "agents", Name: "agents", Type: asset.TypeAgentsDoc,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	if _, addErrs := svc.AddProject("personal", "Nested", target, []string{"codex"}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	_, outcome, applyErr := svc.Apply("personal", "nested", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if _, ok := outcome.(appapi.Committed); !ok {
		t.Fatalf("expected Committed, got %T (%+v)", outcome, outcome)
	}
	files := realHeadFiles(t, outer)
	wantAgents := "target/AGENTS.md"
	wantState := "target/.agentfiles/state.json"
	sawAgents := false
	sawState := false
	for _, f := range files {
		if f == wantAgents {
			sawAgents = true
		}
		if f == wantState {
			sawState = true
		}
	}
	if !sawAgents || !sawState {
		t.Fatalf("HEAD files = %v, want both %q and %q", files, wantAgents, wantState)
	}
}

func isShortSHA(s string) bool {
	if len(s) < 4 || len(s) > 16 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// Package-level compile-time assertion that the fake committer really
// implements the interface; a missing method would surface here rather
// than at a caller line.
var _ GitCommitter = (*fakeCommitter)(nil)

// Silence unused imports when git is absent — the smoke references
// keep the package compilable in that config.
var _ = git.NotARepoError{}
