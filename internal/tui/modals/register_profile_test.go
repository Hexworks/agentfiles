package modals

import (
	"testing"

	"charm.land/huh/v2"
)

func TestRegisterProfile_ResolvesWithTypedInput(t *testing.T) {
	form, state := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})

	state.Path = "/home/addamsson/profiles/demo"
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "register-profile", form, func(*huh.Form) any {
		return *state
	})

	got, ok := msg.Value.(RegisterProfileInput)
	if !ok {
		t.Fatalf("Value type = %T, want RegisterProfileInput", msg.Value)
	}
	if got.Path != "/home/addamsson/profiles/demo" {
		t.Errorf("Path = %q", got.Path)
	}
}

func TestRegisterProfile_CancelResolvesEmpty(t *testing.T) {
	form, state := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})
	abortForm(form)

	msg := runResolvedThroughModal(t, "register-profile", form, func(*huh.Form) any {
		return *state
	})

	if msg.Confirmed || msg.Value != nil {
		t.Errorf("cancel resolved = %#v", msg)
	}
}

func TestNewRegisterProfile_UsesStableID(t *testing.T) {
	m := NewRegisterProfile(RegisterProfileInput{})
	if got := m.ID(); got != "register-profile" {
		t.Errorf("ID = %q, want %q", got, "register-profile")
	}
}
