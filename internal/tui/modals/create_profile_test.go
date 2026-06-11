package modals

import (
	"testing"

	"charm.land/huh/v2"
)

func TestCreateProfile_ResolvesWithTypedInput(t *testing.T) {
	form, state := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

	state.Name = "renamed"
	state.Path = "/tmp/renamed"
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-profile", form, func(*huh.Form) any {
		return *state
	})

	if !msg.Confirmed {
		t.Fatalf("Confirmed = false, want true")
	}
	got, ok := msg.Value.(CreateProfileInput)
	if !ok {
		t.Fatalf("Value type = %T, want CreateProfileInput", msg.Value)
	}
	if got != (CreateProfileInput{Name: "renamed", Path: "/tmp/renamed"}) {
		t.Errorf("Value = %#v", got)
	}
}

func TestCreateProfile_CancelResolvesEmpty(t *testing.T) {
	form, state := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

	abortForm(form)

	msg := runResolvedThroughModal(t, "create-profile", form, func(*huh.Form) any {
		return *state
	})

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
