package modals

import (
	"testing"
)

func TestRegisterProfile_PrefillSeedsState(t *testing.T) {
	_, state, _ := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})
	if state.Path != "/seed" {
		t.Errorf("Path = %q", state.Path)
	}
}

func TestRegisterProfile_PumpResolvesWithTypedInput(t *testing.T) {
	form, _, extract := buildRegisterProfile(RegisterProfileInput{Path: "/home/addamsson/profiles/demo"})
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "register-profile", form, extract)

	got, ok := msg.Value.(RegisterProfileInput)
	if !ok {
		t.Fatalf("Value type = %T, want RegisterProfileInput", msg.Value)
	}
	if got.Path != "/home/addamsson/profiles/demo" {
		t.Errorf("Path = %q", got.Path)
	}
}

func TestRegisterProfile_RejectsEmptyRequiredField(t *testing.T) {
	form, _, _ := buildRegisterProfile(RegisterProfileInput{})
	expectFormStuck(t, form)
}

func TestRegisterProfile_CancelResolvesEmpty(t *testing.T) {
	form, _, extract := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})
	abortForm(form)

	msg := runResolvedThroughModal(t, "register-profile", form, extract)

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
