package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/utils"
)

type harness struct {
	home         string
	v1Registry   string
	profileStore *registry.Store
	projectStore *projectstore.Store
	logged       []string
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

func (h *harness) log(msg string) { h.logged = append(h.logged, msg) }

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
	reg := registry.Registry{
		Version: 1,
		Profiles: []registry.ProfileRef{{
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
	existing := registry.Registry{}
	if data, err := os.ReadFile(h.v1Registry); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	existing.Version = 1
	existing.Profiles = append(existing.Profiles, registry.ProfileRef{
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

	got, err := h.projectStore.Load(map[string]struct{}{"prof-a": {}})
	if err != nil {
		t.Fatalf("load v2 projects: %v", err)
	}
	if len(got["prof-a"]) != 1 || got["prof-a"][0].ID != "repo-a" {
		t.Fatalf("expected migrated project, got %+v", got)
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
	// Both v1 and v2 present. v2 must win: v1 is untouched.
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
	// Seed a registry entry whose Path does not exist.
	reg := registry.Registry{
		Version: 1,
		Profiles: []registry.ProfileRef{{
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
	for _, msg := range h.logged {
		if strings.Contains(msg, "stale profile path") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'stale profile path' warning, got %v", h.logged)
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
	got, err := h.projectStore.Load(map[string]struct{}{"prof-a": {}, "prof-b": {}})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got["prof-a"]) != 1 || len(got["prof-b"]) != 1 {
		t.Fatalf("expected 1 project per profile, got %+v", got)
	}
}
