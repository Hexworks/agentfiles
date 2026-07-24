package modals

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/project"
)

func TestRegisterProject_PrefillSeedsState(t *testing.T) {
	built := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode},
	})
	if built.State.Name != "Demo" || built.State.Path != "/repos/demo" ||
		!reflect.DeepEqual(built.State.EnabledAgents, []string{AgentClaudeCode}) {
		t.Errorf("state = %+v", built.State)
	}
}

func TestRegisterProject_PumpResolvesAsValidManifest(t *testing.T) {
	built := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode, AgentCodex},
	})
	submitForm(t, built.Form)

	msg := runResolvedThroughModal(t, "register-project", built.Form, built.Extract)

	got, ok := msg.Value.(*project.Manifest)
	if !ok {
		t.Fatalf("Value type = %T, want *project.Manifest", msg.Value)
	}
	if got.Name != "Demo" || got.Path != "/repos/demo" {
		t.Errorf("name/path = %q / %q", got.Name, got.Path)
	}
	// MultiSelect.Blur canonicalises the slice to match the option order
	// declared in config.AllAgents.
	if !reflect.DeepEqual(config.AgentStrings(got.EnabledAgents), []string{AgentCodex, AgentClaudeCode}) {
		t.Errorf("EnabledAgents = %v", got.EnabledAgents)
	}
	if got.ID != "demo" {
		t.Errorf("ID = %q, want %q (slug of Name)", got.ID, "demo")
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero; project.NewDraft must stamp it")
	}
	if len(got.SelectedAssetIDs) != 0 {
		t.Errorf("SelectedAssetIDs = %v, want empty", got.SelectedAssetIDs)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("returned manifest fails Validate(): %v", err)
	}
}

func TestRegisterProject_RejectsEmptyRequiredFields(t *testing.T) {
	built := buildRegisterProject(RegisterProjectInput{})
	expectFormStuck(t, built.Form)
}

func TestRegisterProject_RejectsEmptyEnabledAgents(t *testing.T) {
	built := buildRegisterProject(RegisterProjectInput{
		Name: "Demo",
		Path: "/repos/demo",
	})
	expectFormStuck(t, built.Form)
}

func TestRegisterProject_CancelResolvesEmpty(t *testing.T) {
	built := buildRegisterProject(RegisterProjectInput{
		Name: "x", Path: "/p", EnabledAgents: []string{AgentCodex},
	})
	abortForm(built.Form)

	msg := runResolvedThroughModal(t, "register-project", built.Form, built.Extract)

	if msg.Confirmed || msg.Value != nil {
		t.Errorf("cancel resolved = %#v", msg)
	}
}

func TestNewRegisterProject_UsesStableID(t *testing.T) {
	m := NewRegisterProject(RegisterProjectInput{})
	if got := m.ID(); got != "register-project" {
		t.Errorf("ID = %q, want %q", got, "register-project")
	}
}

// TestBuildRegisterProject_PathFieldIsNote — two-step flow contract
// (tasks 0039, 0040): the project root renders as a huh.Note between
// Name and EnabledAgents, not as an editable Input. The rendered view
// must include the picked path so the user can confirm it.
func TestBuildRegisterProject_PathFieldIsNote(t *testing.T) {
	built := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/seed",
		EnabledAgents: []string{AgentClaudeCode},
	})

	if _, ok := built.Fields[1].(*huh.Note); !ok {
		t.Fatalf("fields[1] type = %T, want *huh.Note", built.Fields[1])
	}
	if _, ok := built.Fields[1].(*huh.Input); ok {
		t.Fatalf("fields[1] is *huh.Input; task 0040 requires *huh.Note")
	}
	built.Form.Init()
	if view := built.Form.View(); !strings.Contains(view, "/repos/seed") {
		t.Errorf("form view missing picked path %q; got:\n%s", "/repos/seed", view)
	}
}

// TestBuildRegisterProject_RuneOnMultiSelectLeavesEnabledAgentsUnchanged
// — task 0040 multi-field case: after focus advances past the Name
// Input (skipping the Note, whose Skip() is true when it is not the
// sole field), focus lands on the EnabledAgents MultiSelect. A rune
// that is not part of the MultiSelect keymap (Toggle=`space|x`,
// Filter=`/`, SelectAll=`ctrl+a`, etc.) must not mutate
// state.EnabledAgents — this is the "no key bleed-through" guarantee
// for the multi-field shape. The pre-review test asserted state.Path
// and state.EnabledAgents while focus was still on Name; that
// combination was unfalsifiable by the Note swap.
func TestBuildRegisterProject_RuneOnMultiSelectLeavesEnabledAgentsUnchanged(t *testing.T) {
	seedAgents := []string{AgentClaudeCode}
	built := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/seed",
		EnabledAgents: seedAgents,
	})
	built.Form.Init()

	// Advance focus off Name onto EnabledAgents (Note is skipped in a
	// multi-field group).
	_, cmd := built.Form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainCmd(built.Form, cmd)

	// `z` is not bound in the MultiSelect keymap; it must be ignored.
	_, cmd = built.Form.Update(tea.KeyPressMsg{Text: "z", Code: 'z'})
	drainCmd(built.Form, cmd)

	if !reflect.DeepEqual(built.State.EnabledAgents, seedAgents) {
		t.Errorf("state.EnabledAgents = %v, want %v (unbound rune must not mutate MultiSelect)",
			built.State.EnabledAgents, seedAgents)
	}
}

// TestBuildRegisterProject_EnterCompletesForm — task 0040: feeding real
// tea.KeyPressMsg{Code: tea.KeyEnter} events must drive the form to
// StateCompleted. Enter on Name → NextField (validate ok) → focus on
// EnabledAgents; Enter on EnabledAgents → NextField (validate ok,
// non-empty seed) → nextGroup → completed. The submitForm-based variant
// bypassed the KeyPressMsg dispatch chain and could not have caught the
// regression.
func TestBuildRegisterProject_EnterCompletesForm(t *testing.T) {
	built := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/seed",
		EnabledAgents: []string{AgentClaudeCode},
	})
	pumpEnterUntilCompleted(t, built.Form)
}
