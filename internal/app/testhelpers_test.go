package app

import (
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
)

// newSvc is the shared test factory that assembles the two centralized
// stores under root. Every service-level test uses it so the store
// wiring stays in one place; the app.New signature can then evolve
// without a fan-out edit across every table-driven test.
func newSvc(root string) *Service {
	return New(
		registry.NewStore(filepath.Join(root, "registry.json")),
		projectstore.NewStore(filepath.Join(root, "projects.json")),
	)
}

// newSvcTB is the t.TempDir()-driven variant used by tests that do not
// need to reach for root separately.
func newSvcTB(t *testing.T) *Service {
	t.Helper()
	return newSvc(t.TempDir())
}
