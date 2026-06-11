package modals

import (
	"reflect"
	"testing"
)

func TestEditProject_PrefillSeedsState(t *testing.T) {
	_, state, _ := buildEditProject(EditProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode, AgentCodex},
	})

	if state.Name != "Demo" || state.Path != "/repos/demo" ||
		!reflect.DeepEqual(state.EnabledAgents, []string{AgentClaudeCode, AgentCodex}) {
		t.Errorf("state = %+v", state)
	}
}

func TestEditProject_PrefillCopiesEnabledAgentsSlice(t *testing.T) {
	original := []string{AgentClaudeCode}
	_, state, _ := buildEditProject(EditProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: original,
	})

	state.EnabledAgents[0] = AgentCodex
	if original[0] != AgentClaudeCode {
		t.Errorf("mutating state.EnabledAgents leaked into the caller's slice")
	}
}

func TestEditProject_PumpResolvesAsEditProjectInput(t *testing.T) {
	form, _, extract := buildEditProject(EditProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode, AgentCodex},
	})
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "edit-project", form, extract)

	got, ok := msg.Value.(EditProjectInput)
	if !ok {
		t.Fatalf("Value type = %T, want EditProjectInput", msg.Value)
	}
	// MultiSelect.Blur canonicalises the slice to match the option order
	// declared in config.AllAgents.
	want := EditProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentCodex, AgentClaudeCode},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Value = %#v\nwant   %#v", got, want)
	}
}

func TestEditProject_RejectsEmptyRequiredFields(t *testing.T) {
	form, _, _ := buildEditProject(EditProjectInput{})
	expectFormStuck(t, form)
}

func TestEditProject_RejectsEmptyEnabledAgents(t *testing.T) {
	form, _, _ := buildEditProject(EditProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: nil,
	})
	expectFormStuck(t, form)
}

func TestEditProject_CancelResolvesEmpty(t *testing.T) {
	form, _, extract := buildEditProject(EditProjectInput{
		Name:          "Demo",
		Path:          "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode},
	})
	abortForm(form)

	msg := runResolvedThroughModal(t, "edit-project", form, extract)

	if msg.Confirmed || msg.Value != nil {
		t.Errorf("cancel resolved = %#v", msg)
	}
}

func TestNewEditProject_UsesStableID(t *testing.T) {
	m := NewEditProject(EditProjectInput{Name: "n", Path: "/p", EnabledAgents: []string{AgentCodex}})
	if got := m.ID(); got != "edit-project" {
		t.Errorf("ID = %q, want %q", got, "edit-project")
	}
}
