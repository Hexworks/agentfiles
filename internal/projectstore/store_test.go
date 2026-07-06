package projectstore

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/project"
)

func newManifest(id, name, path string) *project.Manifest {
	return &project.Manifest{
		ID:            id,
		Name:          name,
		Path:          path,
		EnabledAgents: []string{"codex"},
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
	out, err := s.Load(knownSet())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty state, got %d groups", len(out))
	}
}

func TestAddLoadRoundTrip(t *testing.T) {
	s := newStore(t)
	repoDir := t.TempDir()
	m := newManifest("repo", "Repo", repoDir)
	if err := s.Add("prof-a", m); err != nil {
		t.Fatalf("add: %v", err)
	}
	out, err := s.Load(knownSet("prof-a"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out["prof-a"]) != 1 || out["prof-a"][0].ID != "repo" {
		t.Fatalf("expected round-trip repo, got %+v", out)
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
	got, err := s.Load(knownSet("prof-a", "prof-b"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := got["prof-a"]; ok {
		t.Fatalf("prof-a group should be gone")
	}
	if len(got["prof-b"]) != 1 {
		t.Fatalf("prof-b untouched, got %+v", got["prof-b"])
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
