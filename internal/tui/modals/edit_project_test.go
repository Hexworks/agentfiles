package modals

import (
	"reflect"
	"testing"
	"time"

	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/project"
)

func TestEditProject_ResolvesWithModifiedManifest(t *testing.T) {
	existing := &project.Manifest{
		ID:               "demo",
		Name:             "Demo",
		Path:             "/repos/demo",
		EnabledAgents:    []string{AgentClaudeCode},
		SelectedAssetIDs: []string{"asset-a", "asset-b"},
		CreatedAt:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	form, state := buildEditProject(existing)

	state.Name = "Demo Renamed"
	state.Path = "/repos/demo-2"
	state.EnabledAgents = []string{AgentCodex, AgentClaudeCode}
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "edit-project", form, func(*huh.Form) any {
		updated := *existing
		updated.Name = state.Name
		updated.Path = state.Path
		updated.EnabledAgents = state.EnabledAgents
		return &updated
	})

	got, ok := msg.Value.(*project.Manifest)
	if !ok {
		t.Fatalf("Value type = %T, want *project.Manifest", msg.Value)
	}
	if got.ID != "demo" {
		t.Errorf("ID changed to %q; must stay %q", got.ID, "demo")
	}
	if got.Name != "Demo Renamed" || got.Path != "/repos/demo-2" {
		t.Errorf("name/path = %q / %q", got.Name, got.Path)
	}
	if !reflect.DeepEqual(got.SelectedAssetIDs, existing.SelectedAssetIDs) {
		t.Errorf("SelectedAssetIDs = %v, want %v", got.SelectedAssetIDs, existing.SelectedAssetIDs)
	}
	if !got.CreatedAt.Equal(existing.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, existing.CreatedAt)
	}
}

func TestEditProject_RoundTripUnchangedManifest(t *testing.T) {
	existing := &project.Manifest{
		ID:               "demo",
		Name:             "Demo",
		Path:             "/repos/demo",
		EnabledAgents:    []string{AgentClaudeCode, AgentCodex},
		SelectedAssetIDs: []string{"a"},
		CreatedAt:        time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
	}
	form, state := buildEditProject(existing)

	submitForm(t, form)

	msg := runResolvedThroughModal(t, "edit-project", form, func(*huh.Form) any {
		updated := *existing
		updated.Name = state.Name
		updated.Path = state.Path
		updated.EnabledAgents = state.EnabledAgents
		return &updated
	})

	got := msg.Value.(*project.Manifest)
	if !reflect.DeepEqual(got, existing) {
		t.Errorf("round-trip diff:\n got = %#v\nwant = %#v", got, existing)
	}
	if got == existing {
		t.Errorf("returned pointer must be a clone, not the same address")
	}
}

func TestEditProject_CancelResolvesEmpty(t *testing.T) {
	existing := &project.Manifest{
		ID: "demo", Name: "Demo", Path: "/repos/demo",
		EnabledAgents: []string{AgentClaudeCode},
	}
	form, state := buildEditProject(existing)
	abortForm(form)

	msg := runResolvedThroughModal(t, "edit-project", form, func(*huh.Form) any {
		updated := *existing
		updated.Name = state.Name
		updated.Path = state.Path
		updated.EnabledAgents = state.EnabledAgents
		return &updated
	})

	if msg.Confirmed || msg.Value != nil {
		t.Errorf("cancel resolved = %#v", msg)
	}
}

func TestNewEditProject_UsesStableID(t *testing.T) {
	m := NewEditProject(&project.Manifest{Name: "n", Path: "/p", EnabledAgents: []string{AgentCodex}})
	if got := m.ID(); got != "edit-project" {
		t.Errorf("ID = %q, want %q", got, "edit-project")
	}
}
