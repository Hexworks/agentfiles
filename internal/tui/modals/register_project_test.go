package modals

import (
	"reflect"
	"testing"

	"github.com/hexworks/agentfiles/internal/project"
)

func TestRegisterProject_PrefillSeedsState(t *testing.T) {
	_, state, _ := buildRegisterProject(RegisterProjectInput{
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
	form, _, extract := buildRegisterProject(RegisterProjectInput{
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
	form, _, _ := buildRegisterProject(RegisterProjectInput{})
	expectFormStuck(t, form)
}

func TestRegisterProject_RejectsEmptyEnabledAgents(t *testing.T) {
	form, _, _ := buildRegisterProject(RegisterProjectInput{
		Name: "Demo",
		Path: "/repos/demo",
	})
	expectFormStuck(t, form)
}

func TestRegisterProject_CancelResolvesEmpty(t *testing.T) {
	form, _, extract := buildRegisterProject(RegisterProjectInput{
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
