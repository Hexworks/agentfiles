package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/styles"
)

func TestCreateProfile_PrefillSeedsState(t *testing.T) {
	built := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

	if built.State.Name != "demo" || built.State.Path != "/tmp/demo" {
		t.Errorf("state = %+v", built.State)
	}
}

func TestCreateProfile_PumpResolvesWithTypedInput(t *testing.T) {
	built := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})
	submitForm(t, built.Form)

	msg := runResolvedThroughModal(t, "create-profile", built.Form, built.Extract)

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
	built := buildCreateProfile(CreateProfileInput{})
	expectFormStuck(t, built.Form)
}

func TestCreateProfile_CancelResolvesEmpty(t *testing.T) {
	built := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/demo"})

	abortForm(built.Form)

	msg := runResolvedThroughModal(t, "create-profile", built.Form, built.Extract)

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
	built := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/seed"})

	if _, ok := built.Fields[1].(*huh.Note); !ok {
		t.Fatalf("fields[1] type = %T, want *huh.Note", built.Fields[1])
	}
	if _, ok := built.Fields[1].(*huh.Input); ok {
		t.Fatalf("fields[1] is *huh.Input; task 0040 requires *huh.Note")
	}
	built.Form.Init()
	if view := built.Form.View(); !strings.Contains(view, "/tmp/seed") {
		t.Errorf("form view missing picked path %q; got:\n%s", "/tmp/seed", view)
	}
}

// TestBuildCreateProfile_RuneKeyLeavesPathUnchanged — task 0040
// symptom: with a huh.Input the picked path was mutated by any keypress
// reaching the field. Focus after Init lands on Name (an Input), so the
// rune mutates state.Name; the path row is a Note with no value binding
// and state.Path must stay untouched regardless.
func TestBuildCreateProfile_RuneKeyLeavesPathUnchanged(t *testing.T) {
	built := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/seed"})
	built.Form.Init()

	_, cmd := built.Form.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	drainCmd(built.Form, cmd)

	if built.State.Path != "/tmp/seed" {
		t.Errorf("state.Path = %q, want %q (rune must not mutate a Note)", built.State.Path, "/tmp/seed")
	}
}

// TestBuildCreateProfile_EnterCompletesForm — task 0040: with a valid
// Name seeded, feeding a real tea.KeyPressMsg{Code: tea.KeyEnter}
// through form.Update must drive the group to completion. Name.Update
// validates and returns NextField; Group.nextField walks past the Note
// (Skip()==true when it is not the sole field) and hits OnLast, which
// emits nextGroup and completes the form. The submitForm-based variant
// exercised nextFieldMsg directly and could not have caught the
// regression in Note.Update's keypress path.
func TestBuildCreateProfile_EnterCompletesForm(t *testing.T) {
	built := buildCreateProfile(CreateProfileInput{Name: "demo", Path: "/tmp/seed"})
	pumpEnterUntilCompleted(t, built.Form)
}

// TestPathDisplayNote_SoleFieldEnterCompletesForm validates
// Note.Update's Enter branch in the "sole field in its group" shape —
// the exact position pathDisplayNote occupies in register-profile.
// AC 6 wording was aimed at this behaviour but is not achievable in
// create-profile's multi-field group (Note.Skip() returns true when the
// Note is not sole, so focus never lands on it there). This test uses a
// bespoke single-Note form to pin the same guarantee down.
func TestPathDisplayNote_SoleFieldEnterCompletesForm(t *testing.T) {
	form := huh.NewForm(
		huh.NewGroup(pathDisplayNote("/tmp/seed", "Picked path")),
	).WithTheme(styles.HuhTheme())
	form.Init()

	_, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainCmd(form, cmd)

	if form.State != huh.StateCompleted {
		t.Fatalf("form.State = %v, want StateCompleted", form.State)
	}
}
