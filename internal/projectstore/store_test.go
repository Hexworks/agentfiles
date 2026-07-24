package projectstore

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/project"
)

func newManifest(id, name, path string) *project.Manifest {
	return &project.Manifest{
		ID:            id,
		Name:          name,
		Path:          path,
		EnabledAgents: []agent.Agent{agent.Codex},
		CreatedAt:     time.Now().UTC(),
	}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), config.ProjectsStoreFileName))
}

func knownSet(ids ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func TestLoad_MissingFileIsEmpty(t *testing.T) {
	s := newStore(t)
	state, err := s.Load(knownSet())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if state == nil || len(state.Projects) != 0 {
		t.Fatalf("expected empty state, got %+v", state)
	}
}

func TestAddLoadRoundTrip(t *testing.T) {
	s := newStore(t)
	repoDir := t.TempDir()
	m := newManifest("repo", "Repo", repoDir)
	if err := s.Add("prof-a", m); err != nil {
		t.Fatalf("add: %v", err)
	}
	state, err := s.Load(knownSet("prof-a"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(state.Projects["prof-a"]) != 1 || state.Projects["prof-a"][0].ID != "repo" {
		t.Fatalf("expected round-trip repo, got %+v", state)
	}
}

func TestAddRejectsDuplicate(t *testing.T) {
	s := newStore(t)
	repoDir := t.TempDir()
	m := newManifest("repo", "Repo", repoDir)
	if err := s.Add("prof-a", m); err != nil {
		t.Fatalf("add first: %v", err)
	}
	err := s.Add("prof-a", newManifest("repo", "Repo2", t.TempDir()))
	var dup DuplicateProjectIDError
	if !errors.As(err, &dup) {
		t.Fatalf("expected DuplicateProjectIDError, got %T: %v", err, err)
	}
}

func TestAdd_RejectsForeignPathOwned(t *testing.T) {
	s := newStore(t)
	shared := t.TempDir()
	if err := s.Add("prof-a", newManifest("repo-a", "Repo A", shared)); err != nil {
		t.Fatalf("add first: %v", err)
	}
	err := s.Add("prof-b", newManifest("repo-b", "Repo B", shared))
	var owned ProjectPathOwnedError
	if !errors.As(err, &owned) {
		t.Fatalf("expected ProjectPathOwnedError, got %T: %v", err, err)
	}
	if owned.ExistingProfileID != "prof-a" || owned.ExistingProjectID != "repo-a" {
		t.Fatalf("expected conflict against (prof-a/repo-a), got %+v", owned)
	}
}

func TestUpdate_RejectsForeignPathOwned(t *testing.T) {
	s := newStore(t)
	foreign := t.TempDir()
	own := t.TempDir()
	if err := s.Add("prof-a", newManifest("repo-a", "Repo A", foreign)); err != nil {
		t.Fatalf("add prof-a: %v", err)
	}
	if err := s.Add("prof-b", newManifest("repo-b", "Repo B", own)); err != nil {
		t.Fatalf("add prof-b: %v", err)
	}
	moving := newManifest("repo-b", "Repo B", foreign)
	err := s.Update("prof-b", moving)
	var owned ProjectPathOwnedError
	if !errors.As(err, &owned) {
		t.Fatalf("expected ProjectPathOwnedError, got %T: %v", err, err)
	}
}

func TestUpdate(t *testing.T) {
	s := newStore(t)
	m := newManifest("repo", "Repo", t.TempDir())
	if err := s.Add("prof-a", m); err != nil {
		t.Fatalf("add: %v", err)
	}
	m.Name = "Repo v2"
	if err := s.Update("prof-a", m); err != nil {
		t.Fatalf("update: %v", err)
	}
	out, _ := s.ListByProfile("prof-a")
	if out[0].Name != "Repo v2" {
		t.Fatalf("expected updated name, got %q", out[0].Name)
	}
}

func TestUpdate_MissingProject(t *testing.T) {
	s := newStore(t)
	err := s.Update("prof-a", newManifest("ghost", "Ghost", t.TempDir()))
	var nf ProjectNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
}

func TestRemove(t *testing.T) {
	s := newStore(t)
	m := newManifest("repo", "Repo", t.TempDir())
	if err := s.Add("prof-a", m); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.Remove("prof-a", "repo"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	out, _ := s.ListByProfile("prof-a")
	if len(out) != 0 {
		t.Fatalf("expected empty group, got %d", len(out))
	}
}

func TestRemoveByProfile_CascadesEveryProject(t *testing.T) {
	s := newStore(t)
	if err := s.Add("prof-a", newManifest("r1", "R1", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("prof-a", newManifest("r2", "R2", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("prof-b", newManifest("r3", "R3", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveByProfile("prof-a"); err != nil {
		t.Fatalf("remove by profile: %v", err)
	}
	state, err := s.Load(knownSet("prof-a", "prof-b"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := state.Projects["prof-a"]; ok {
		t.Fatalf("prof-a group should be gone")
	}
	if len(state.Projects["prof-b"]) != 1 {
		t.Fatalf("prof-b untouched, got %+v", state.Projects["prof-b"])
	}
}

func TestRemoveByProfile_UnknownProfileIsIdempotent(t *testing.T) {
	s := newStore(t)
	if err := s.RemoveByProfile("nobody"); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestListByProfile_SortedByName(t *testing.T) {
	s := newStore(t)
	if err := s.Add("prof-a", newManifest("b", "Beta", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("prof-a", newManifest("a", "Alpha", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	out, err := s.ListByProfile("prof-a")
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Name != "Alpha" || out[1].Name != "Beta" {
		t.Fatalf("expected [Alpha, Beta], got %+v", out)
	}
}

// TestSave_WritesProjectsSortedByName asserts the Save-time sort so a
// regression that dropped it would fail here (ListByProfile also sorts
// on read and would mask the regression on its own).
func TestSave_WritesProjectsSortedByName(t *testing.T) {
	s := newStore(t)
	if err := s.Save(&State{Version: Version, Projects: map[string][]*project.Manifest{
		"prof-a": {
			newManifest("b", "Beta", t.TempDir()),
			newManifest("a", "Alpha", t.TempDir()),
		},
	}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Read via a fresh store instance without invoking ListByProfile so
	// we observe the on-disk order directly.
	fresh := NewStore(s.Path)
	state, err := fresh.readState()
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	group := state.Projects["prof-a"]
	if len(group) != 2 || group[0].Name != "Alpha" || group[1].Name != "Beta" {
		t.Fatalf("expected on-disk [Alpha, Beta], got %+v", group)
	}
}

func TestAllProjects_SortedDeterministic(t *testing.T) {
	s := newStore(t)
	if err := s.Add("prof-b", newManifest("z", "Zulu", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("prof-a", newManifest("a", "Alpha", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("prof-a", newManifest("b", "Beta", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got, err := s.AllProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 owned projects, got %d", len(got))
	}
	if got[0].ProfileID != "prof-a" || got[0].Manifest.Name != "Alpha" {
		t.Fatalf("expected prof-a/Alpha first, got %+v", got[0])
	}
	if got[1].ProfileID != "prof-a" || got[1].Manifest.Name != "Beta" {
		t.Fatalf("expected prof-a/Beta second, got %+v", got[1])
	}
	if got[2].ProfileID != "prof-b" || got[2].Manifest.Name != "Zulu" {
		t.Fatalf("expected prof-b/Zulu last, got %+v", got[2])
	}
}

// TestAllProjects_SingleProfileSortedByName covers the majority case: a
// single-group user expects deterministic ordering too. Regression to
// insertion-order would only fail here.
func TestAllProjects_SingleProfileSortedByName(t *testing.T) {
	s := newStore(t)
	if err := s.Add("prof-a", newManifest("b", "Beta", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("prof-a", newManifest("a", "Alpha", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got, err := s.AllProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Manifest.Name != "Alpha" || got[1].Manifest.Name != "Beta" {
		t.Fatalf("expected [Alpha, Beta] in one group, got %+v", got)
	}
}

func TestLoad_ReportsOrphanProfileID(t *testing.T) {
	s := newStore(t)
	if err := s.Add("gone", newManifest("r", "R", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	_, err := s.Load(knownSet())
	var orphan OrphanProfileIDError
	if !errors.As(err, &orphan) {
		t.Fatalf("expected OrphanProfileIDError, got %T: %v", err, err)
	}
	if orphan.ProfileID != "gone" {
		t.Fatalf("expected profile id 'gone', got %q", orphan.ProfileID)
	}
}

// TestCRUD_RunsOrphanCheckThroughKnownProvider asserts the wired
// KnownProvider surfaces orphan groups on internal CRUD reads, not only
// on Load. A hand-edited projects.json with a group under an unknown id
// would otherwise sit there quietly and block a legitimate Add.
func TestCRUD_RunsOrphanCheckThroughKnownProvider(t *testing.T) {
	s := newStore(t)
	// Seed a projects.json with an orphan group via Save (bypassing the
	// orphan gate, matching a hand edit or a stale registry).
	if err := s.Save(&State{Version: Version, Projects: map[string][]*project.Manifest{
		"gone": {newManifest("r", "R", t.TempDir())},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.KnownProfiles = func() (map[string]struct{}, errs.DomainError) {
		return knownSet(), nil
	}
	err := s.Add("prof-a", newManifest("new", "New", t.TempDir()))
	var orphan OrphanProfileIDError
	if !errors.As(err, &orphan) {
		t.Fatalf("expected orphan surfaced via CRUD, got %T: %v", err, err)
	}
}
