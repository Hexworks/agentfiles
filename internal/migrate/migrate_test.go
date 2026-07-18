package migrate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/utils"
)

type loggedEvent struct {
	severity errs.Severity
	msg      string
}

type harness struct {
	home         string
	v1Registry   string
	profileStore *registry.Store
	projectStore *projectstore.Store
	logged       []loggedEvent
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := &harness{
		home:         home,
		v1Registry:   filepath.Join(home, v1RegistryFileName),
		profileStore: registry.NewStore(filepath.Join(home, config.UserConfigDirName, config.ProfilesStoreFileName)),
		projectStore: projectstore.NewStore(filepath.Join(home, config.UserConfigDirName, config.ProjectsStoreFileName)),
	}
	return h
}

func (h *harness) log(severity errs.Severity, msg string) {
	h.logged = append(h.logged, loggedEvent{severity: severity, msg: msg})
}

func (h *harness) seedV1Profile(t *testing.T, id, name, rel string, projects []*project.Manifest) {
	t.Helper()
	profileRoot := filepath.Join(h.home, rel)
	if err := os.MkdirAll(profileRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if err := utils.WriteJSON(filepath.Join(profileRoot, v1ProjectsDirName, p.ID+".json"), p); err != nil {
			t.Fatalf("seed project: %v", err)
		}
	}
	reg := v1Registry{
		Version: 1,
		Profiles: []v1ProfileRef{{
			ID:   id,
			Name: name,
			Path: profileRoot,
		}},
	}
	body, _ := json.MarshalIndent(reg, "", "  ")
	if err := os.WriteFile(h.v1Registry, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) mergeSeed(t *testing.T, id, name, rel string, projects []*project.Manifest) {
	t.Helper()
	profileRoot := filepath.Join(h.home, rel)
	if err := os.MkdirAll(profileRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if err := utils.WriteJSON(filepath.Join(profileRoot, v1ProjectsDirName, p.ID+".json"), p); err != nil {
			t.Fatalf("seed project: %v", err)
		}
	}
	existing := v1Registry{}
	if data, err := os.ReadFile(h.v1Registry); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	existing.Version = 1
	existing.Profiles = append(existing.Profiles, v1ProfileRef{
		ID:   id,
		Name: name,
		Path: profileRoot,
	})
	body, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(h.v1Registry, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func manifest(id, name, path string) *project.Manifest {
	return &project.Manifest{
		ID:            id,
		Name:          name,
		Path:          path,
		EnabledAgents: []string{"codex"},
	}
}

func TestRun_HappyPathWritesV2AndDeletesV1(t *testing.T) {
	h := newHarness(t)
	h.seedV1Profile(t, "prof-a", "Personal", "profiles/personal",
		[]*project.Manifest{manifest("repo-a", "Repo A", filepath.Join(h.home, "repo-a"))})

	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !utils.Exists(h.profileStore.Path) {
		t.Fatalf("expected v2 profiles.json at %s", h.profileStore.Path)
	}
	if !utils.Exists(h.projectStore.Path) {
		t.Fatalf("expected v2 projects.json at %s", h.projectStore.Path)
	}
	if utils.Exists(h.v1Registry) {
		t.Fatalf("expected v1 registry removed at %s", h.v1Registry)
	}
	if utils.Exists(filepath.Join(h.home, "profiles/personal", v1ProjectsDirName)) {
		t.Fatalf("expected v1 projects dir removed")
	}

	state, err := h.projectStore.Load(map[string]struct{}{"prof-a": {}})
	if err != nil {
		t.Fatalf("load v2 projects: %v", err)
	}
	if len(state.Projects["prof-a"]) != 1 || state.Projects["prof-a"][0].ID != "repo-a" {
		t.Fatalf("expected migrated project, got %+v", state)
	}
}

func TestRun_IdempotentSecondRun(t *testing.T) {
	h := newHarness(t)
	h.seedV1Profile(t, "prof-a", "Personal", "profiles/personal",
		[]*project.Manifest{manifest("repo-a", "Repo A", filepath.Join(h.home, "repo-a"))})

	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run 2: %v", err)
	}
}

func TestRun_V2PresentIsNoOp(t *testing.T) {
	h := newHarness(t)
	h.seedV1Profile(t, "prof-a", "Personal", "profiles/personal", nil)
	if err := utils.WriteJSON(h.profileStore.Path, registry.Registry{Version: 1}); err != nil {
		t.Fatalf("seed v2: %v", err)
	}

	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !utils.Exists(h.v1Registry) {
		t.Fatalf("expected v1 registry left intact when v2 already present")
	}
}

func TestRun_MissingV1IsNoOp(t *testing.T) {
	h := newHarness(t)
	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run: %v", err)
	}
	if utils.Exists(h.profileStore.Path) {
		t.Fatalf("expected no v2 profiles.json when v1 missing")
	}
	if utils.Exists(h.projectStore.Path) {
		t.Fatalf("expected no v2 projects.json when v1 missing")
	}
}

func TestRun_StaleProfilePathSkippedWithWarning(t *testing.T) {
	h := newHarness(t)
	reg := v1Registry{
		Version: 1,
		Profiles: []v1ProfileRef{{
			ID:   "gone",
			Name: "Gone",
			Path: filepath.Join(h.home, "no-such-dir"),
		}},
	}
	body, _ := json.MarshalIndent(reg, "", "  ")
	if err := os.WriteFile(h.v1Registry, body, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(h.logged) == 0 {
		t.Fatalf("expected at least one log entry")
	}
	found := false
	for _, ev := range h.logged {
		if ev.severity != errs.SeverityWarning {
			continue
		}
		if !strings.HasPrefix(ev.msg, "stale profile path skipped: ") {
			continue
		}
		if !strings.Contains(ev.msg, "no-such-dir") {
			continue
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("expected warning-severity 'stale profile path skipped: …no-such-dir…' entry, got %+v", h.logged)
	}
}

func TestRun_MultipleProfilesGroupedByID(t *testing.T) {
	h := newHarness(t)
	h.seedV1Profile(t, "prof-a", "A", "profiles/a",
		[]*project.Manifest{manifest("repo-a", "A", filepath.Join(h.home, "repo-a"))})
	h.mergeSeed(t, "prof-b", "B", "profiles/b",
		[]*project.Manifest{manifest("repo-b", "B", filepath.Join(h.home, "repo-b"))})

	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run: %v", err)
	}
	state, err := h.projectStore.Load(map[string]struct{}{"prof-a": {}, "prof-b": {}})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(state.Projects["prof-a"]) != 1 || len(state.Projects["prof-b"]) != 1 {
		t.Fatalf("expected 1 project per profile, got %+v", state)
	}
}

// TestRun_ProjectStoreSaveFailureLeavesV1Intact simulates the projects.json
// write failing (target dir made unwritable) and asserts the v1
// originals still exist so a retry after the operator fixes the target
// directory converges.
func TestRun_ProjectStoreSaveFailureLeavesV1Intact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unwritable simulation not portable on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores 0o500 mode bits")
	}
	h := newHarness(t)
	h.seedV1Profile(t, "prof-a", "Personal", "profiles/personal",
		[]*project.Manifest{manifest("repo-a", "Repo A", filepath.Join(h.home, "repo-a"))})

	userConfigDir := filepath.Dir(h.projectStore.Path)
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(userConfigDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(userConfigDir, 0o755) })

	runErr := Run(h.profileStore, h.projectStore, h.log)
	if runErr == nil {
		t.Fatal("expected run to fail when projects.json write is denied")
	}

	if !utils.Exists(h.v1Registry) {
		t.Fatalf("expected v1 registry left intact on save failure")
	}
	if !utils.Exists(filepath.Join(h.home, "profiles/personal", v1ProjectsDirName)) {
		t.Fatalf("expected v1 projects dir left intact on save failure")
	}
	if utils.Exists(h.profileStore.Path) {
		t.Fatalf("expected no v2 profiles.json when the earlier write failed")
	}
}

// TestRun_ProfileStoreSaveFailureAfterProjectStoreSucceededLeavesRecoverable
// covers the write-then-swap ordering hazard: if projects.json is
// durable but profiles.json is not, the presence gate on the next
// launch must still recognize the layout as post-migration so v1 does
// not get re-harvested and duplicated. We simulate the "projects.json
// written but profiles.json never made it" state by seeding
// projects.json directly and asserting the next Run treats it as
// already-migrated (leaving v1 untouched).
func TestRun_ProfileStoreSaveFailureAfterProjectStoreSucceededLeavesRecoverable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unwritable simulation not portable on windows")
	}
	h := newHarness(t)
	h.seedV1Profile(t, "prof-a", "Personal", "profiles/personal",
		[]*project.Manifest{manifest("repo-a", "Repo A", filepath.Join(h.home, "repo-a"))})
	// Pre-seed projects.json as if a partial migration left it behind
	// before profiles.json was written. The presence gate must recognize
	// this and skip a re-migration so we do not double-harvest.
	if err := os.MkdirAll(filepath.Dir(h.projectStore.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := utils.WriteJSON(h.projectStore.Path, projectstore.State{Version: projectstore.Version}); err != nil {
		t.Fatalf("seed projects.json: %v", err)
	}

	if err := Run(h.profileStore, h.projectStore, h.log); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !utils.Exists(h.v1Registry) {
		t.Fatalf("expected v1 registry left intact when partial v2 present")
	}
	if utils.Exists(h.profileStore.Path) {
		t.Fatalf("expected profiles.json not written when migration skipped")
	}
}

// TestRun_MalformedV1ManifestFailsLoudly asserts that a hand-corrupted
// project manifest inside a v1 projects/ dir stops the migration with a
// domain error instead of silently dropping data.
func TestRun_MalformedV1ManifestFailsLoudly(t *testing.T) {
	h := newHarness(t)
	h.seedV1Profile(t, "prof-a", "Personal", "profiles/personal", nil)
	badPath := filepath.Join(h.home, "profiles/personal", v1ProjectsDirName, "broken.json")
	if err := os.MkdirAll(filepath.Dir(badPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	runErr := Run(h.profileStore, h.projectStore, h.log)
	if runErr == nil {
		t.Fatal("expected malformed manifest to fail the run")
	}
	var readJSONErr utils.ReadJSONError
	if !errors.As(runErr, &readJSONErr) {
		t.Fatalf("expected utils.ReadJSONError, got %T: %v", runErr, runErr)
	}
}
