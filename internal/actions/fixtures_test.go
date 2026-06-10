package actions_test

import (
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
)

// fixture is the shared bag returned by newFixture. ProfilePath is the
// path of the seeded profile (when WithProfile is true); empty
// otherwise.
type fixture struct {
	A           *actions.Actions
	Svc         *app.Service
	Root        string
	ProfilePath string
}

type fixtureOpts struct {
	withProfile bool
	profileName string
}

type fixtureOpt func(*fixtureOpts)

// withProfile seeds a profile named "Personal" (slug "personal") under
// <root>/profile. Tests that want a different name override via
// withProfileName.
func withProfile() fixtureOpt { return func(o *fixtureOpts) { o.withProfile = true } }

func newFixture(t *testing.T, opts ...fixtureOpt) fixture {
	t.Helper()
	o := fixtureOpts{profileName: "Personal"}
	for _, apply := range opts {
		apply(&o)
	}
	root := t.TempDir()
	svc := app.New(filepath.Join(root, "registry.json"))
	f := fixture{
		A:    actions.New(svc),
		Svc:  svc,
		Root: root,
	}
	if o.withProfile {
		f.ProfilePath = filepath.Join(root, "profile")
		if _, err := svc.CreateProfile(o.profileName, f.ProfilePath); err != nil {
			t.Fatalf("seed profile: %v", err)
		}
	}
	return f
}
