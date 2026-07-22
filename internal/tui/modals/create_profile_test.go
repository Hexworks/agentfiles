package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func TestCreateProfile_PrefillSeedsState(t *testing.T) {
	_, state, _, _ := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

	if state.Name != "demo" || state.Path != "/tmp/demo" {
		t.Errorf("state = %+v", state)
	}
}

func TestCreateProfile_PumpResolvesWithTypedInput(t *testing.T) {
	form, _, extract, _ := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})
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
	form, _, _, _ := buildCreateProfile(CreateProfileInput{})
	expectFormStuck(t, form)
}

func TestCreateProfile_CancelResolvesEmpty(t *testing.T) {
	form, _, extract, _ := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

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

// TestBuildCreateProfile_PathFieldIsNote — two-step flow contract
// (tasks 0039, 0040): the picked path renders as a huh.Note in the
// second (Name) slot, not an editable Input. The rendered view must
// include the picked path so the user can confirm it.
func TestBuildCreateProfile_PathFieldIsNote(t *testing.T) {
	form, _, _, fields := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/seed"})

	if _, ok := fields[1].(*huh.Note); !ok {
		t.Fatalf("fields[1] type = %T, want *huh.Note", fields[1])
	}
	if _, ok := fields[1].(*huh.Input); ok {
		t.Fatalf("fields[1] is *huh.Input; task 0040 requires *huh.Note")
	}
	form.Init()
	if view := form.View(); !strings.Contains(view, "/tmp/seed") {
		t.Errorf("form view missing picked path %q; got:\n%s", "/tmp/seed", view)
	}
}

// TestBuildCreateProfile_RuneKeyLeavesPathUnchanged — task 0040
// symptom: with a huh.Input the path was mutated by any keypress reaching
// the field. Note has no value binding, so a rune keypress must leave
// state.Path untouched regardless of which field currently has focus.
func TestBuildCreateProfile_RuneKeyLeavesPathUnchanged(t *testing.T) {
	form, state, _, _ := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/seed"})
	form.Init()

	drainCmd(form, func() tea.Cmd {
		_, cmd := form.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
		return cmd
	}())

	if state.Path != "/tmp/seed" {
		t.Errorf("state.Path = %q, want %q (rune must not mutate a Note)", state.Path, "/tmp/seed")
	}
}

// TestBuildCreateProfile_EnterCompletesForm — task 0040: with a valid
// Name seeded, driving the form to completion (Enter cycles through
// Name → Note → nextGroup) must reach StateCompleted. The Note is the
// last field in a multi-field group, so Note.Skip() is true and the
// group hits OnLast without focus visibly landing on the path row.
func TestBuildCreateProfile_EnterCompletesForm(t *testing.T) {
	form, _, _, _ := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/seed"})
	submitForm(t, form)

	if form.State != huh.StateCompleted {
		t.Fatalf("form.State = %v, want StateCompleted", form.State)
	}
}
