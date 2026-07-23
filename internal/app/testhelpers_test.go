package app

import (
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/settings"
)

// recordedCommit captures a Commit call made by fakeCommitter so tests
// can assert every git-aware trigger produced the expected (dir,
// pathspec, msg) tuple. The struct is exported inside the test package
// so table-driven tests can compare via reflect.DeepEqual.
type recordedCommit struct {
	Dir      string
	Pathspec []string
	Msg      string
}

// fakeCommitter is the GitCommitter injected into every service test.
// SHA / Err drive the fake's return; Calls records every invocation.
// Zero-value returns ("", nil) — a silent skip — which is safe for tests
// that do not care about the commit path.
type fakeCommitter struct {
	SHA   string
	Err   errs.DomainError
	Calls []recordedCommit
}

func (f *fakeCommitter) Commit(dir string, pathspec []string, msg string) (string, errs.DomainError) {
	f.Calls = append(f.Calls, recordedCommit{
		Dir:      dir,
		Pathspec: append([]string(nil), pathspec...),
		Msg:      msg,
	})
	return f.SHA, f.Err
}

// newSvc is the shared test factory that assembles the three centralized
// stores under root. Every service-level test uses it so the store
// wiring stays in one place; the NewWithStores signature can then
// evolve without a fan-out edit across every table-driven test.
func newSvc(root string) *Service {
	return newSvcWith(root, settings.Default(), &fakeCommitter{})
}

// newSvcWith is newSvc with an explicit initial Settings and committer,
// used by tests that exercise the git-aware paths.
func newSvcWith(root string, s settings.Settings, committer GitCommitter) *Service {
	return NewWithStores(
		registry.NewStore(filepath.Join(root, "registry.json")),
		projectstore.NewStore(filepath.Join(root, "projects.json")),
		settings.NewStore(filepath.Join(root, "settings.json")),
		s,
		committer,
	)
}

// newSvcTB is the t.TempDir()-driven variant used by tests that do not
// need to reach for root separately.
func newSvcTB(t *testing.T) *Service {
	t.Helper()
	return newSvc(t.TempDir())
}
