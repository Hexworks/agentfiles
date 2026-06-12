package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// stubScreenAccessors is the smallest surface every back-only nav stub
// exposes for tests: title, captured ids, status keys, and a body
// renderer. The table below drives the shared behavior assertions
// without forcing each stub into its own file.
type stubScreenAccessors interface {
	Screen
	ProfileID() string
}

func TestNavigationStubs_SharedBackBehavior(t *testing.T) {
	cases := []struct {
		name      string
		title     string
		construct func() (stub stubScreenAccessors, targetID string)
		targetTag string // word that must appear in Body alongside the ids
	}{
		{
			name:  "edit asset",
			title: "Edit Asset",
			construct: func() (stubScreenAccessors, string) {
				s := newEditAssetStub("alpha-123", "skill-xyz")
				return s, s.AssetID()
			},
			targetTag: "skill-xyz",
		},
		{
			name:  "select project assets",
			title: "Select Project Assets",
			construct: func() (stubScreenAccessors, string) {
				s := newSelectProjectAssetsStub("alpha-123", "proj-xyz")
				return s, s.ProjectID()
			},
			targetTag: "proj-xyz",
		},
		{
			name:  "plan project",
			title: "Plan Project",
			construct: func() (stubScreenAccessors, string) {
				s := newPlanProjectStub("alpha-123", "proj-xyz")
				return s, s.ProjectID()
			},
			targetTag: "proj-xyz",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, targetID := tc.construct()

			if got := s.Title(); got != tc.title {
				t.Errorf("Title() = %q, want %q", got, tc.title)
			}

			// b + esc both trigger pop.
			for _, kp := range []tea.KeyPressMsg{
				{Code: 'b', Text: "b"},
				{Code: tea.KeyEsc},
			} {
				_, cmd := s.Update(kp)
				if cmd == nil {
					t.Fatalf("%v produced nil cmd", kp)
				}
				if _, ok := cmd().(PopScreenMsg); !ok {
					t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
				}
			}

			// unrelated key is a no-op.
			if _, cmd := s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd != nil {
				t.Errorf("unrelated key produced cmd = %v, want nil", cmd)
			}

			// StatusKeys exposes Back with the correct help shape.
			keys := s.StatusKeys()
			if len(keys) != 1 {
				t.Fatalf("StatusKeys length = %d, want 1", len(keys))
			}
			h := keys[0].Help()
			if h.Key != "b" || h.Desc != "Back" {
				t.Errorf("Help() = (%q, %q), want (b, Back)", h.Key, h.Desc)
			}

			// Body carries both ids.
			body := s.Body(80)
			for _, want := range []string{s.ProfileID(), targetID, tc.targetTag} {
				if !strings.Contains(body, want) {
					t.Errorf("Body missing %q\n%s", want, body)
				}
			}
		})
	}
}

func TestNavigationStubs_EmptyIDsPanic(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"editAssetStub empty profile", func() { newEditAssetStub("", "x") }},
		{"editAssetStub empty asset", func() { newEditAssetStub("x", "") }},
		{"selectProjectAssetsStub empty profile", func() { newSelectProjectAssetsStub("", "x") }},
		{"selectProjectAssetsStub empty project", func() { newSelectProjectAssetsStub("x", "") }},
		{"planProjectStub empty profile", func() { newPlanProjectStub("", "x") }},
		{"planProjectStub empty project", func() { newPlanProjectStub("x", "") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("expected panic, got none")
				}
			}()
			tc.fn()
		})
	}
}

// Body renders at its natural height: sentence + spacer + back row = 3.
func TestNavigationStubs_BodyHasNaturalHeight(t *testing.T) {
	stubs := []Screen{
		newEditAssetStub("a", "b"),
		newSelectProjectAssetsStub("a", "b"),
		newPlanProjectStub("a", "b"),
	}
	for _, s := range stubs {
		body := s.Body(60)
		got := strings.Count(body, "\n") + 1
		if got != 3 {
			t.Errorf("%T Body height = %d, want 3\n%s", s, got, body)
		}
	}
}

// Compile-time guards that the three stubs satisfy the Screen contract.
var (
	_ Screen = (*editAssetStub)(nil)
	_ Screen = (*selectProjectAssetsStub)(nil)
	_ Screen = (*planProjectStub)(nil)
)
