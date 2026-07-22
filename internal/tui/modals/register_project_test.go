package modals

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/project"
)

func TestRegisterProject_PrefillSeedsState(t *testing.T) {
	_, state, _, _ := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode},
	})
	if state.Name != "Demo" || state.Path != "/repos/demo" ||
		!reflect.DeepEqual(state.EnabledAgents, []string{AgentClaudeCode}) {
		t.Errorf("state = %+v", state)
	}
}

func TestRegisterProject_PumpResolvesAsValidManifest(t *testing.T) {
	form, _, extract, _ := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode, AgentCodex},
	})
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "register-project", form, extract)

	got, ok := msg.Value.(*project.Manifest)
	if !ok {
		t.Fatalf("Value type = %T, want *project.Manifest", msg.Value)
	}
	if got.Name != "Demo" || got.Path != "/repos/demo" {
		t.Errorf("name/path = %q / %q", got.Name, got.Path)
	}
	// MultiSelect.Blur canonicalises the slice to match the option order
	// declared in config.AllAgents.
	if !reflect.DeepEqual(got.EnabledAgents, []string{AgentCodex, AgentClaudeCode}) {
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
	form, _, _, _ := buildRegisterProject(RegisterProjectInput{})
	expectFormStuck(t, form)
}

func TestRegisterProject_RejectsEmptyEnabledAgents(t *testing.T) {
	form, _, _, _ := buildRegisterProject(RegisterProjectInput{
		Name: "Demo",
		Path: "/repos/demo",
	})
	expectFormStuck(t, form)
}

func TestRegisterProject_CancelResolvesEmpty(t *testing.T) {
	form, _, extract, _ := buildRegisterProject(RegisterProjectInput{
		Name: "x", Path: "/p", EnabledAgents: []string{AgentCodex},
	})
	abortForm(form)

	msg := runResolvedThroughModal(t, "register-project", form, extract)

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
	form, _, _, fields := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/seed",
		EnabledAgents: []string{AgentClaudeCode},
	})

	if _, ok := fields[1].(*huh.Note); !ok {
		t.Fatalf("fields[1] type = %T, want *huh.Note", fields[1])
	}
	if _, ok := fields[1].(*huh.Input); ok {
		t.Fatalf("fields[1] is *huh.Input; task 0040 requires *huh.Note")
	}
	form.Init()
	if view := form.View(); !strings.Contains(view, "/repos/seed") {
		t.Errorf("form view missing picked path %q; got:\n%s", "/repos/seed", view)
	}
}

// TestBuildRegisterProject_RuneKeyLeavesStateUnchanged — task 0040:
// with a huh.Input the picked path was mutated by any keypress. Note
// has no value binding, so runes fed to the focused form must leave
// state.Path AND state.EnabledAgents untouched (no key bleed-through in
// the multi-field case either).
func TestBuildRegisterProject_RuneKeyLeavesStateUnchanged(t *testing.T) {
	seedAgents := []string{AgentClaudeCode}
	form, state, _, _ := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/seed",
		EnabledAgents: seedAgents,
	})
	form.Init()

	drainCmd(form, func() tea.Cmd {
		_, cmd := form.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
		return cmd
	}())

	if state.Path != "/repos/seed" {
		t.Errorf("state.Path = %q, want %q (rune must not mutate a Note)", state.Path, "/repos/seed")
	}
	if !reflect.DeepEqual(state.EnabledAgents, seedAgents) {
		t.Errorf("state.EnabledAgents = %v, want %v (rune must not leak into MultiSelect)", state.EnabledAgents, seedAgents)
	}
}

// TestBuildRegisterProject_EnterCompletesForm — task 0040: with a
// valid Name + EnabledAgents seed, driving the form through submitForm
// must reach StateCompleted. The Note is not the sole field so
// Note.Skip() is true; the group walks Name → (skip Note) →
// EnabledAgents → nextGroup and completes.
func TestBuildRegisterProject_EnterCompletesForm(t *testing.T) {
	form, _, _, _ := buildRegisterProject(RegisterProjectInput{
		Name:          "Demo",
		Path:          "/repos/seed",
		EnabledAgents: []string{AgentClaudeCode},
	})
	submitForm(t, form)

	if form.State != huh.StateCompleted {
		t.Fatalf("form.State = %v, want StateCompleted", form.State)
	}
}
