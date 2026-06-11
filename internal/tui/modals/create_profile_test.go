package modals

import (
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
