package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func TestRegisterProfile_PrefillSeedsState(t *testing.T) {
	built := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})
	if built.State.Path != "/seed" {
		t.Errorf("Path = %q", built.State.Path)
	}
}

func TestRegisterProfile_PumpResolvesWithTypedInput(t *testing.T) {
	built := buildRegisterProfile(RegisterProfileInput{Path: "/home/addamsson/profiles/demo"})
	submitForm(t, built.Form)

	msg := runResolvedThroughModal(t, "register-profile", built.Form, built.Extract)

	got, ok := msg.Value.(RegisterProfileInput)
	if !ok {
		t.Fatalf("Value type = %T, want RegisterProfileInput", msg.Value)
	}
	if got.Path != "/home/addamsson/profiles/demo" {
		t.Errorf("Path = %q", got.Path)
	}
}

func TestRegisterProfile_CancelResolvesEmpty(t *testing.T) {
	built := buildRegisterProfile(RegisterProfileInput{Path: "/seed"})
	abortForm(built.Form)

	msg := runResolvedThroughModal(t, "register-profile", built.Form, built.Extract)

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
	built := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})

	if _, ok := built.Fields[0].(*huh.Note); !ok {
		t.Fatalf("fields[0] type = %T, want *huh.Note", built.Fields[0])
	}
	if _, ok := built.Fields[0].(*huh.Input); ok {
		t.Fatalf("fields[0] is *huh.Input; task 0040 requires *huh.Note")
	}
	built.Form.Init()
	if view := built.Form.View(); !strings.Contains(view, "/tmp/seed") {
		t.Errorf("form view missing picked path %q; got:\n%s", "/tmp/seed", view)
	}
}

// TestBuildRegisterProfile_RuneKeyLeavesPathUnchanged — task 0040
// symptom: with a huh.Input the path was mutated by any keypress. Note
// has no value binding, so a rune keypress on the focused (sole) Note
// must leave state.Path untouched.
func TestBuildRegisterProfile_RuneKeyLeavesPathUnchanged(t *testing.T) {
	built := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})
	built.Form.Init()

	_, cmd := built.Form.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	drainCmd(built.Form, cmd)

	if built.State.Path != "/tmp/seed" {
		t.Errorf("state.Path = %q, want %q (rune must not mutate a Note)", built.State.Path, "/tmp/seed")
	}
}

// TestBuildRegisterProfile_EnterCompletesForm — task 0040 root symptom:
// Enter on the single-field group leaked out of the form and re-opened
// the pathselector. Feeding a real tea.KeyPressMsg{Code: tea.KeyEnter}
// through form.Update must route through Group.Update → Note.Update and
// end in StateCompleted; the earlier submitForm-based variant of this
// test bypassed that path via nextFieldMsg and could not have caught the
// regression.
func TestBuildRegisterProfile_EnterCompletesForm(t *testing.T) {
	built := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})
	built.Form.Init()

	_, cmd := built.Form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainCmd(built.Form, cmd)

	if built.Form.State != huh.StateCompleted {
		t.Fatalf("form.State = %v, want StateCompleted", built.Form.State)
	}
}

// TestBuildRegisterProfile_EnterConsumedNoLeak is the load-bearing task
// 0040 regression guard: in the single-Note group (the shape where the
// bug lived) an Enter keypress must be consumed by the form. If any raw
// tea.KeyPressMsg reappears in the drained message stream, the shell
// would see it and re-trigger the "select path" action — exactly the
// leak that re-opened the pathselector before this fix.
func TestBuildRegisterProfile_EnterConsumedNoLeak(t *testing.T) {
	built := buildRegisterProfile(RegisterProfileInput{Path: "/tmp/seed"})
	built.Form.Init()

	_, cmd := built.Form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drained := drainCmdCollect(built.Form, cmd)

	if built.Form.State != huh.StateCompleted {
		t.Fatalf("form.State = %v, want StateCompleted", built.Form.State)
	}
	for _, m := range drained {
		if _, ok := m.(tea.KeyPressMsg); ok {
			t.Fatalf("Enter keypress leaked back into the drained message stream (%T); form did not consume it", m)
		}
	}
}
