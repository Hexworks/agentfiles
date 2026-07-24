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
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/git"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/settings"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
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
	if _, addErrs := svc.AddProject("personal", "Repo", repoPath, []config.Agent{config.AgentCodex}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	out, applyErr := svc.Apply("personal", "repo", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if got := mustCommitted(t, out.Sync); got.SHA != "sync001" {
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
		[]config.Agent{config.AgentCodex}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	out, applyErr := svc.Apply("personal", "repo", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	mustSkipped(t, out.Sync, appapi.SkipDisabled)
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
	if _, addErrs := svc.AddProject("personal", "Repo", targetRepo, []config.Agent{config.AgentCodex}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	out, applyErr := svc.Apply("personal", "repo", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if _, ok := out.Sync.(appapi.Committed); !ok {
		t.Fatalf("expected Committed, got %T (%+v)", out.Sync, out.Sync)
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
	if _, addErrs := svc.AddProject("personal", "Nested", target, []config.Agent{config.AgentCodex}, []string{"agents"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}

	out, applyErr := svc.Apply("personal", "nested", appapi.Resolutions{})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if _, ok := out.Sync.(appapi.Committed); !ok {
		t.Fatalf("expected Committed, got %T (%+v)", out.Sync, out.Sync)
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

// seedAdoptProject seeds a fake-committer service with a skill asset
// "foo" (SKILL.md), a project pointing at repoPath, and a first Apply
// that lays down .claude/skills/foo/SKILL.md plus its v3 state.json.
// The returned skill body is the last-applied managed content.
func seedAdoptProject(t *testing.T, s settings.Settings, fc *fakeCommitter) (svc *Service, profileID, projectID, repoPath, skillBody string) {
	t.Helper()
	root := t.TempDir()
	svc = newSvcWith(root, s, fc)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "foo", Name: "foo", Type: asset.TypeSkill, Description: "seed",
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	repoPath = filepath.Join(root, "repo")
	if _, addErrs := svc.AddProject("personal", "Repo", repoPath, []config.Agent{config.AgentClaudeCode}, []string{"foo"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}
	if _, applyErr := svc.Apply("personal", "repo", appapi.Resolutions{}); applyErr != nil {
		t.Fatalf("initial apply: %v", applyErr)
	}
	// Snapshot the last-applied SKILL.md body for the drift step.
	got, readErr := os.ReadFile(filepath.Join(repoPath, ".claude", "skills", "foo", "SKILL.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	skillBody = string(got)
	fc.Calls = nil
	return svc, "personal", "repo", repoPath, skillBody
}

// TestService_Apply_AdoptCommitsProfileWhenGitEnabled pins the ADR
// 0020 profile-side commit: after a DriftAdopt resolution the fake
// committer sees a second call scoped to the profile repo with the
// adopt subject.
func TestService_Apply_AdoptCommitsProfileWhenGitEnabled(t *testing.T) {
	fc := &fakeCommitter{SHA: "adopt01"}
	svc, profileID, projectID, repoPath, _ := seedAdoptProject(t, enabledSettings, fc)
	// Edit the rendered file locally to create drift.
	skillRepo := filepath.Join(repoPath, ".claude", "skills", "foo", "SKILL.md")
	if err := os.WriteFile(skillRepo, []byte("adopted body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, applyErr := svc.Apply(profileID, projectID, appapi.Resolutions{
		Drift: []appapi.DriftResolution{{Path: ".claude/skills/foo/SKILL.md", Decision: appapi.DriftAdopt}},
	})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	// Two commit calls: one on the target repo (sync), one on the
	// profile repo (adopt).
	if len(fc.Calls) != 2 {
		t.Fatalf("committer calls = %d, want 2", len(fc.Calls))
	}
	// out.Sync is the target-repo commit result; out.Adopt is the
	// profile-repo one. Both come out Committed because SHA is set on
	// the fake.
	if _, ok := out.Sync.(appapi.Committed); !ok {
		t.Fatalf("out.Sync = %T, want Committed", out.Sync)
	}
	if got := mustCommitted(t, out.Adopt); got.SHA != "adopt01" {
		t.Fatalf("adopt SHA = %q, want adopt01", got.SHA)
	}
	// Second call carries the adopt template + pathspec pointing at
	// the profile asset file.
	adoptCall := fc.Calls[1]
	if !strings.HasPrefix(adoptCall.Msg, "chore(agentfiles): adopt ") || !strings.Contains(adoptCall.Msg, "file(s) into profile") {
		t.Fatalf("adopt msg = %q, want adopt subject template", adoptCall.Msg)
	}
	if len(adoptCall.Pathspec) != 1 || !strings.HasSuffix(adoptCall.Pathspec[0], "/SKILL.md") {
		t.Fatalf("adopt pathspec = %v, want single SKILL.md entry", adoptCall.Pathspec)
	}
}

// TestService_Apply_AdoptSkipsCommitWhenGitDisabled pins the disabled
// path: the asset file still gets written, but no commit is attempted.
func TestService_Apply_AdoptSkipsCommitWhenGitDisabled(t *testing.T) {
	fc := &fakeCommitter{}
	svc, profileID, projectID, repoPath, _ := seedAdoptProject(t, settings.Default(), fc)
	skillRepo := filepath.Join(repoPath, ".claude", "skills", "foo", "SKILL.md")
	if err := os.WriteFile(skillRepo, []byte("adopted disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, applyErr := svc.Apply(profileID, projectID, appapi.Resolutions{
		Drift: []appapi.DriftResolution{{Path: ".claude/skills/foo/SKILL.md", Decision: appapi.DriftAdopt}},
	})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if len(fc.Calls) != 0 {
		t.Fatalf("committer called %d times, want 0", len(fc.Calls))
	}
	mustSkipped(t, out.Adopt, appapi.SkipDisabled)
	// Adopt still copies the body into the profile asset even when
	// the git commit is off.
	profileAsset := loadedAsset(t, svc, profileID, "foo")
	got, readErr := os.ReadFile(filepath.Join(profileAsset.Dir, "SKILL.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "adopted disabled\n" {
		t.Fatalf("profile SKILL.md = %q, want adopted body", got)
	}
}

// TestService_Apply_AdoptWritesAssetFileWithProvenance pins that the
// profile-side file matches the repo edit byte-for-byte and the repo
// file is untouched by adopt.
func TestService_Apply_AdoptWritesAssetFileWithProvenance(t *testing.T) {
	fc := &fakeCommitter{}
	svc, profileID, projectID, repoPath, _ := seedAdoptProject(t, settings.Default(), fc)
	skillRepo := filepath.Join(repoPath, ".claude", "skills", "foo", "SKILL.md")
	edited := "adopted provenance body\n"
	if err := os.WriteFile(skillRepo, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, applyErr := svc.Apply(profileID, projectID, appapi.Resolutions{
		Drift: []appapi.DriftResolution{{Path: ".claude/skills/foo/SKILL.md", Decision: appapi.DriftAdopt}},
	}); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	profileAsset := loadedAsset(t, svc, profileID, "foo")
	profileBody, err := os.ReadFile(filepath.Join(profileAsset.Dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(profileBody) != edited {
		t.Fatalf("profile SKILL.md = %q, want %q", profileBody, edited)
	}
	repoBody, err := os.ReadFile(skillRepo)
	if err != nil {
		t.Fatal(err)
	}
	if string(repoBody) != edited {
		t.Fatalf("repo SKILL.md changed: %q", repoBody)
	}
}

// TestService_Apply_AdoptMissingAssetSurfacesError pins the
// accumulator contract inside executeAdoptRequests: an AdoptRequest
// whose AssetID no longer resolves in the loaded profile produces a
// typed error and no commit lands.
func TestService_Apply_AdoptMissingAssetSurfacesError(t *testing.T) {
	fc := &fakeCommitter{}
	svc, _, _, _, _ := seedAdoptProject(t, enabledSettings, fc)
	loaded, err := svc.LoadProfile("personal")
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	proj := loaded.Projects["repo"]
	fc.Calls = nil

	// Hand-craft a bogus AdoptRequest that references an asset id the
	// loaded profile has no entry for. executeAdoptRequests must
	// surface an AdoptTargetMissingError and never invoke the fake.
	outcome, failures := svc.executeAdoptRequests(loaded, proj, []llmsync.AdoptRequest{
		{Path: "AGENTS.md", AssetID: "bogus", SourceRel: "AGENTS.md"},
	})
	mustSkipped(t, outcome, appapi.SkipDisabled)
	if len(failures) != 1 {
		t.Fatalf("failures = %v, want single AdoptTargetMissingError", failures)
	}
	var missing AdoptTargetMissingError
	if !errors.As(failures[0], &missing) {
		t.Fatalf("expected AdoptTargetMissingError, got %T: %v", failures[0], failures[0])
	}
	if missing.AssetID != "bogus" || missing.Path != "AGENTS.md" {
		t.Fatalf("AdoptTargetMissingError = %+v, want {AGENTS.md bogus}", missing)
	}
	if len(fc.Calls) != 0 {
		t.Fatalf("committer called %d times, want 0", len(fc.Calls))
	}
}

// TestService_Apply_AdoptRepoFileMissingSurfacesReadError pins the
// AdoptReadError accumulator contract: if the repo-side source file
// disappears between Plan and Apply the failure is typed and does not
// roll back peer adopts in the same batch. See task 0035 review issue
// #12.
func TestService_Apply_AdoptRepoFileMissingSurfacesReadError(t *testing.T) {
	fc := &fakeCommitter{}
	svc, profileID, _, repoPath, _ := seedAdoptProject(t, settings.Default(), fc)
	// Seed a second skill so we have a peer adopt that must succeed
	// even when the first fails.
	if _, err := svc.InitAsset(profileID, asset.Manifest{
		ID: "bar", Name: "bar", Type: asset.TypeSkill, Description: "peer",
	}); err != nil {
		t.Fatalf("init peer asset: %v", err)
	}
	// Re-add bar to the project and re-apply so the second skill's
	// state entry lands under v3 with provenance.
	loaded, err := svc.LoadProfile(profileID)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	proj := loaded.Projects["repo"]
	proj.SelectedAssetIDs = append(proj.SelectedAssetIDs, "bar")
	if updateErr := svc.Projects.Update(profileID, proj); updateErr != nil {
		t.Fatalf("update project: %v", updateErr)
	}
	if _, applyErr := svc.Apply(profileID, "repo", appapi.Resolutions{}); applyErr != nil {
		t.Fatalf("re-apply peer: %v", applyErr)
	}
	fc.Calls = nil
	// Edit both skill files so both drift; then remove the first one
	// so its adopt read fails while the second succeeds.
	fooPath := filepath.Join(repoPath, ".claude", "skills", "foo", "SKILL.md")
	barPath := filepath.Join(repoPath, ".claude", "skills", "bar", "SKILL.md")
	if err := os.WriteFile(fooPath, []byte("foo drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(barPath, []byte("bar drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fooPath); err != nil {
		t.Fatalf("remove foo: %v", err)
	}
	// Rehydrate loaded/proj so we resolve the projects map through
	// the app service rather than a stale pointer.
	loaded2, err := svc.LoadProfile(profileID)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	proj2 := loaded2.Projects["repo"]

	// executeAdoptRequests is the boundary we want to pin. Hand-craft
	// two AdoptRequest values pointing at both drifted files.
	outcome, failures := svc.executeAdoptRequests(loaded2, proj2, []llmsync.AdoptRequest{
		{Path: ".claude/skills/foo/SKILL.md", AssetID: "foo", SourceRel: "SKILL.md"},
		{Path: ".claude/skills/bar/SKILL.md", AssetID: "bar", SourceRel: "SKILL.md"},
	})
	mustSkipped(t, outcome, appapi.SkipDisabled)
	if len(failures) != 1 {
		t.Fatalf("failures = %v, want single AdoptReadError", failures)
	}
	var readErr AdoptReadError
	if !errors.As(failures[0], &readErr) {
		t.Fatalf("expected AdoptReadError, got %T: %v", failures[0], failures[0])
	}
	// Peer adopt succeeded: bar's asset file should carry the drift
	// body even though foo's read failed.
	barAsset := loadedAsset(t, svc, profileID, "bar")
	got, readGotErr := os.ReadFile(filepath.Join(barAsset.Dir, "SKILL.md"))
	if readGotErr != nil {
		t.Fatal(readGotErr)
	}
	if string(got) != "bar drift\n" {
		t.Fatalf("peer bar body = %q, want bar drift after peer adopt", got)
	}
}

// TestService_Apply_Adopt_RealGitRecordsProfileCommit mirrors
// TestApply_RealGitRecordsProjectCommit for the ADR 0020 reverse
// flow: after a DriftAdopt with git enabled the profile repo picks
// up an actual commit whose diff carries the adopted asset file.
// See task 0035 review issue #12.
func TestService_Apply_Adopt_RealGitRecordsProfileCommit(t *testing.T) {
	requireGitBinary(t)
	dataRoot := t.TempDir()
	profileParent := filepath.Join(dataRoot, "profiles")
	targetRepo := filepath.Join(dataRoot, "target")
	if err := os.MkdirAll(profileParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	initRealRepo(t, targetRepo)
	svc := newRealSvc(t, profileParent)
	profileRoot := filepath.Join(profileParent, "personal")
	if _, err := svc.CreateProfile("Personal", profileRoot); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := svc.InitAsset("personal", asset.Manifest{
		ID: "foo", Name: "foo", Type: asset.TypeSkill,
	}); err != nil {
		t.Fatalf("init asset: %v", err)
	}
	// Init the profile repo AFTER scaffolding so the seed commit
	// covers profile.json + the SKILL.md asset. Later the adopt
	// commit lands as a diff-only entry.
	initRealRepo(t, profileRoot)
	runShellGit(t, profileRoot, "add", "-A")
	runShellGit(t, profileRoot, "commit", "-q", "-m", "seed profile")
	if _, addErrs := svc.AddProject("personal", "Target", targetRepo, []config.Agent{config.AgentClaudeCode}, []string{"foo"}); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}
	if _, applyErr := svc.Apply("personal", "target", appapi.Resolutions{}); applyErr != nil {
		t.Fatalf("initial apply: %v", applyErr)
	}
	// Edit locally to create drift.
	skillRepo := filepath.Join(targetRepo, ".claude", "skills", "foo", "SKILL.md")
	edited := "real-git adopted body\n"
	if err := os.WriteFile(skillRepo, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	out, applyErr := svc.Apply("personal", "target", appapi.Resolutions{
		Drift: []appapi.DriftResolution{{Path: ".claude/skills/foo/SKILL.md", Decision: appapi.DriftAdopt}},
	})
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if _, ok := out.Adopt.(appapi.Committed); !ok {
		t.Fatalf("out.Adopt = %T, want Committed", out.Adopt)
	}
	files := realHeadFiles(t, profileRoot)
	sawSkill := false
	for _, f := range files {
		if strings.HasSuffix(f, "/SKILL.md") {
			sawSkill = true
			break
		}
	}
	if !sawSkill {
		t.Fatalf("profile HEAD files = %v, want to see /SKILL.md", files)
	}
	profileAsset := loadedAsset(t, svc, "personal", "foo")
	body, err := os.ReadFile(filepath.Join(profileAsset.Dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != edited {
		t.Fatalf("profile SKILL.md = %q, want %q", body, edited)
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
