package modals

import (
	"strings"
	"testing"
)

func TestCreateProfile_PrefillSeedsState(t *testing.T) {
	_, state, _ := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

	if state.Name != "demo" || state.Path != "/tmp/demo" {
		t.Errorf("state = %+v", state)
	}
}

func TestCreateProfile_PumpResolvesWithTypedInput(t *testing.T) {
	form, _, extract := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-profile", form, extract)

	if !msg.Confirmed {
		t.Fatalf("Confirmed = false, want true")
	}
	got, ok := msg.Value.(CreateProfileInput)
	if !ok {
		t.Fatalf("Value type = %T, want CreateProfileInput", msg.Value)
	}
	if got != (CreateProfileInput{Name: "demo", Path: "/tmp/demo"}) {
		t.Errorf("Value = %#v", got)
	}
}

func TestCreateProfile_RejectsEmptyRequiredField(t *testing.T) {
	form, _, _ := buildCreateProfile(CreateProfileInput{})
	expectFormStuck(t, form)
}

func TestCreateProfile_CancelResolvesEmpty(t *testing.T) {
	form, _, extract := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

	abortForm(form)

	msg := runResolvedThroughModal(t, "create-profile", form, extract)

	if msg.Confirmed {
		t.Errorf("Confirmed = true, want false on cancel")
	}
	if msg.Value != nil {
		t.Errorf("Value = %v, want nil on cancel", msg.Value)
	}
}

func TestNewCreateProfile_UsesStableID(t *testing.T) {
	m := NewCreateProfile(CreateProfileInput{})
	if got := m.ID(); got != "create-profile" {
		t.Errorf("ID = %q, want %q", got, "create-profile")
	}
}

// TestBuildCreateProfile_PathReadOnly locks in the two-step flow contract
// (task 0039): the pathselector step supplies the path and the form step
// only displays it. The seeded path must reach the shared state pointer,
// and the field's description must carry the "(read-only)" marker so the
// user recognizes the field as non-editable — huh has no runtime read-only
// mode, so the marker is the only signal.
func TestBuildCreateProfile_PathReadOnly(t *testing.T) {
	form, state, _ := buildCreateProfile(CreateProfileInput{Path: "/tmp/seed"})

	if state.Path != "/tmp/seed" {
		t.Errorf("state.Path = %q, want %q", state.Path, "/tmp/seed")
	}
	form.Init()
	if view := form.View(); !strings.Contains(view, "read-only") {
		t.Errorf("form view missing %q marker; got:\n%s", "read-only", view)
	}
}
