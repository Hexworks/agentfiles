package modals

import (
	"reflect"
	"testing"

	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/project"
)

func TestRegisterProject_ResolvesAsManifest(t *testing.T) {
	form, state := buildRegisterProject(RegisterProjectInput{})

	state.Name = "Demo"
	state.Path = "/repos/demo"
	state.EnabledAgents = []string{AgentClaudeCode, AgentCodex}
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "register-project", form, func(*huh.Form) any {
		return &project.Manifest{
			Name:          state.Name,
			Path:          state.Path,
			EnabledAgents: state.EnabledAgents,
		}
	})

	got, ok := msg.Value.(*project.Manifest)
	if !ok {
		t.Fatalf("Value type = %T, want *project.Manifest", msg.Value)
	}
	if got.Name != "Demo" || got.Path != "/repos/demo" {
		t.Errorf("name/path = %q / %q", got.Name, got.Path)
	}
	if !reflect.DeepEqual(got.EnabledAgents, []string{AgentClaudeCode, AgentCodex}) {
		t.Errorf("EnabledAgents = %v", got.EnabledAgents)
	}
	if got.ID != "" {
		t.Errorf("ID = %q, want empty (slug happens in app.Service.AddProject)", got.ID)
	}
	if len(got.SelectedAssetIDs) != 0 {
		t.Errorf("SelectedAssetIDs = %v, want empty", got.SelectedAssetIDs)
	}
}

func TestRegisterProject_CancelResolvesEmpty(t *testing.T) {
	form, state := buildRegisterProject(RegisterProjectInput{
		Name: "x", Path: "/p", EnabledAgents: []string{AgentCodex},
	})
	abortForm(form)

	msg := runResolvedThroughModal(t, "register-project", form, func(*huh.Form) any {
		return &project.Manifest{
			Name:          state.Name,
			Path:          state.Path,
			EnabledAgents: state.EnabledAgents,
		}
	})

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
