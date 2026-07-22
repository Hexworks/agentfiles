package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func TestRegisterProfile_PrefillSeedsState(t *testing.T) {
	_, state, _, _ := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})
	if state.Path != "/seed" {
		t.Errorf("Path = %q", state.Path)
	}
}

func TestRegisterProfile_PumpResolvesWithTypedInput(t *testing.T) {
	form, _, extract, _ := buildRegisterProfile(RegisterProfileInput{Path: "/home/addamsson/profiles/demo"})
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

func TestRegisterProfile_CancelResolvesEmpty(t *testing.T) {
	form, _, extract, _ := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})
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

// TestBuildRegisterProfile_PathFieldIsNote locks in the two-step flow
// contract (tasks 0039, 0040): the pathselector step supplies the path
// and the register-profile form displays it as a huh.Note — not an
// editable Input. The rendered view must include the picked path so the
// user can confirm it before submitting.
func TestBuildRegisterProfile_PathFieldIsNote(t *testing.T) {
	form, _, _, fields := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})

	if _, ok := fields[0].(*huh.Note); !ok {
		t.Fatalf("fields[0] type = %T, want *huh.Note", fields[0])
	}
	if _, ok := fields[0].(*huh.Input); ok {
		t.Fatalf("fields[0] is *huh.Input; task 0040 requires *huh.Note")
	}
	form.Init()
	if view := form.View(); !strings.Contains(view, "/tmp/seed") {
		t.Errorf("form view missing picked path %q; got:\n%s", "/tmp/seed", view)
	}
}

// TestBuildRegisterProfile_RuneKeyLeavesPathUnchanged — task 0040
// symptom: with a huh.Input the path was mutated by any keypress. Note
// has no value binding, so a rune keypress on the focused form must
// leave state.Path untouched.
func TestBuildRegisterProfile_RuneKeyLeavesPathUnchanged(t *testing.T) {
	form, state, _, _ := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})
	form.Init()

	drainCmd(form, func() tea.Cmd {
		_, cmd := form.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
		return cmd
	}())

	if state.Path != "/tmp/seed" {
		t.Errorf("state.Path = %q, want %q (rune must not mutate a Note)", state.Path, "/tmp/seed")
	}
}

// TestBuildRegisterProfile_EnterCompletesForm — task 0040 root symptom:
// Enter on the single-field group leaked out of the form and re-opened
// the pathselector. Note.Update returns NextField on Enter, so the
// single-Note group must reach StateCompleted.
func TestBuildRegisterProfile_EnterCompletesForm(t *testing.T) {
	form, _, _, _ := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})
	submitForm(t, form)

	if form.State != huh.StateCompleted {
		t.Fatalf("form.State = %v, want StateCompleted", form.State)
	}
}
