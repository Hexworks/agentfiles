package modals

import (
	"reflect"
	"testing"

	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/asset"
)

func TestCreateAsset_ResolvesAsManifest(t *testing.T) {
	form, state := buildCreateAsset(asset.Manifest{})

	state.Name = "Hello World"
	state.Type = asset.TypeAgentsDoc
	state.Description = "test asset"
	state.Tags = "git, build, ci"
	state.CompatibleAgents = []string{AgentClaudeCode, AgentCodex}
	state.ExclusiveGroup = "agents_doc"
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-asset", form, func(*huh.Form) any {
		return assetManifestFromState(state)
	})

	got, ok := msg.Value.(asset.Manifest)
	if !ok {
		t.Fatalf("Value type = %T, want asset.Manifest", msg.Value)
	}

	want := asset.Manifest{
		ID:               "hello-world",
		Name:             "Hello World",
		Type:             asset.TypeAgentsDoc,
		Description:      "test asset",
		Tags:             []string{"git", "build", "ci"},
		CompatibleAgents: []string{AgentClaudeCode, AgentCodex},
		ExclusiveGroup:   "agents_doc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Manifest = %#v\nwant       %#v", got, want)
	}
}

func TestCreateAsset_DefaultsTypeToSkillWhenInitialEmpty(t *testing.T) {
	_, state := buildCreateAsset(asset.Manifest{})
	if state.Type != asset.TypeSkill {
		t.Errorf("default Type = %q, want %q", state.Type, asset.TypeSkill)
	}
}

func TestCreateAsset_OmitsEmptyOptionalFieldsFromManifest(t *testing.T) {
	form, state := buildCreateAsset(asset.Manifest{})
	state.Name = "no-extras"
	state.Type = asset.TypeRule
	state.Description = "minimal"
	state.Tags = "   "          // whitespace only → nil
	state.CompatibleAgents = nil // empty → empty
	state.ExclusiveGroup = ""
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-asset", form, func(*huh.Form) any {
		return assetManifestFromState(state)
	})

	got := msg.Value.(asset.Manifest)
	if got.Tags != nil {
		t.Errorf("Tags = %v, want nil for whitespace-only input", got.Tags)
	}
	if len(got.CompatibleAgents) != 0 {
		t.Errorf("CompatibleAgents = %v, want empty", got.CompatibleAgents)
	}
	if got.ExclusiveGroup != "" {
		t.Errorf("ExclusiveGroup = %q, want empty", got.ExclusiveGroup)
	}
	if got.ID != "no-extras" {
		t.Errorf("ID = %q, want %q", got.ID, "no-extras")
	}
}

func TestCreateAsset_CancelResolvesEmpty(t *testing.T) {
	form, state := buildCreateAsset(asset.Manifest{Name: "x", Description: "y"})
	abortForm(form)

	msg := runResolvedThroughModal(t, "create-asset", form, func(*huh.Form) any {
		return assetManifestFromState(state)
	})

	if msg.Confirmed || msg.Value != nil {
		t.Errorf("cancel resolved = %#v", msg)
	}
}

func TestNewCreateAsset_UsesStableID(t *testing.T) {
	m := NewCreateAsset(asset.Manifest{})
	if got := m.ID(); got != "create-asset" {
		t.Errorf("ID = %q, want %q", got, "create-asset")
	}
}

func TestNewCreateAsset_PrefillRoundtripsTags(t *testing.T) {
	_, state := buildCreateAsset(asset.Manifest{
		Tags: []string{"a", "b", "c"},
	})
	if state.Tags != "a, b, c" {
		t.Errorf("Tags csv = %q, want %q", state.Tags, "a, b, c")
	}
}
