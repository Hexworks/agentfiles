package modals

import (
	"reflect"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
)

func TestCreateAsset_PrefillSeedsState(t *testing.T) {
	_, state, _ := buildCreateAsset(asset.Manifest{
		Name:             "Existing",
		Type:             asset.TypeAgentsDoc,
		Description:      "desc",
		Tags:             []string{"a", "b"},
		CompatibleAgents: []string{AgentCodex},
		ExclusiveGroup:   "g",
	}, asset.AllTypes())

	if state.Name != "Existing" || state.Type != asset.TypeAgentsDoc ||
		state.Description != "desc" || state.Tags != "a, b" ||
		!reflect.DeepEqual(state.CompatibleAgents, []string{AgentCodex}) ||
		state.ExclusiveGroup != "g" {
		t.Errorf("state = %+v", state)
	}
}

func TestCreateAsset_PumpResolvesAsManifestWithoutID(t *testing.T) {
	form, _, extract := buildCreateAsset(asset.Manifest{
		Name:             "Hello World",
		Type:             asset.TypeAgentsDoc,
		Description:      "test asset",
		Tags:             []string{"git", "build", "ci"},
		CompatibleAgents: []string{AgentClaudeCode, AgentCodex},
		ExclusiveGroup:   "agents_doc",
	}, asset.AllTypes())
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-asset", form, extract)

	got, ok := msg.Value.(asset.Manifest)
	if !ok {
		t.Fatalf("Value type = %T, want asset.Manifest", msg.Value)
	}
	// MultiSelect.Blur canonicalises the slice to match the option order
	// declared in config.AllAgents, so claude-code/codex come back as
	// codex/claude-code.
	want := asset.Manifest{
		Name:             "Hello World",
		Type:             asset.TypeAgentsDoc,
		Description:      "test asset",
		Tags:             []string{"git", "build", "ci"},
		CompatibleAgents: []string{AgentCodex, AgentClaudeCode},
		ExclusiveGroup:   "agents_doc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Manifest = %#v\nwant       %#v", got, want)
	}
	if got.ID != "" {
		t.Errorf("ID = %q, want empty (slug applied by app.Service.InitAsset)", got.ID)
	}
}

func TestCreateAsset_RejectsEmptyRequiredFields(t *testing.T) {
	form, _, _ := buildCreateAsset(asset.Manifest{}, asset.AllTypes())
	expectFormStuck(t, form)
}

func TestCreateAsset_OmitsEmptyOptionalFieldsFromManifest(t *testing.T) {
	form, _, extract := buildCreateAsset(asset.Manifest{
		Name:        "no-extras",
		Type:        asset.TypeRule,
		Description: "minimal",
	}, asset.AllTypes())
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-asset", form, extract)

	got := msg.Value.(asset.Manifest)
	if got.Tags != nil {
		t.Errorf("Tags = %v, want nil for unset input", got.Tags)
	}
	if len(got.CompatibleAgents) != 0 {
		t.Errorf("CompatibleAgents = %v, want empty", got.CompatibleAgents)
	}
	if got.ExclusiveGroup != "" {
		t.Errorf("ExclusiveGroup = %q, want empty", got.ExclusiveGroup)
	}
	if got.Name != "no-extras" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestCreateAsset_CancelResolvesEmpty(t *testing.T) {
	form, _, extract := buildCreateAsset(asset.Manifest{Name: "x", Description: "y"}, asset.AllTypes())
	abortForm(form)

	msg := runResolvedThroughModal(t, "create-asset", form, extract)

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

func TestCreateAsset_PrefillRoundtripsTags(t *testing.T) {
	_, state, _ := buildCreateAsset(asset.Manifest{
		Tags: []string{"a", "b", "c"},
	}, asset.AllTypes())
	if state.Tags != "a, b, c" {
		t.Errorf("Tags csv = %q, want %q", state.Tags, "a, b, c")
	}
}
