package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/utils"
)

// createAssetState is the in-flight form state shared by the field bindings
// and the extract closure. It mirrors `asset.Manifest` but stores tags as
// the user-facing comma-separated string so the input field can bind to it
// directly.
type createAssetState struct {
	Name             string
	Type             asset.Type
	Description      string
	Tags             string
	CompatibleAgents []string
	ExclusiveGroup   string
}

// NewCreateAsset builds the Create Asset modal. `initial` lets callers
// preload fields (also used by tests). The resulting payload is an
// `asset.Manifest` ready for `app.Service.InitAsset`; the screen is
// responsible for any domain-level retry on `AssetExistsError`.
func NewCreateAsset(initial asset.Manifest) *modal.Modal {
	form, state := buildCreateAsset(initial)
	return modal.NewForm("create-asset", form, func(*huh.Form) any {
		return assetManifestFromState(state)
	})
}

func buildCreateAsset(initial asset.Manifest) (*huh.Form, *createAssetState) {
	state := &createAssetState{
		Name:             initial.Name,
		Type:             initial.Type,
		Description:      initial.Description,
		Tags:             joinTags(initial.Tags),
		CompatibleAgents: append([]string(nil), initial.CompatibleAgents...),
		ExclusiveGroup:   initial.ExclusiveGroup,
	}
	if state.Type == "" {
		state.Type = asset.TypeSkill
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("name").
				Description("The name of the asset (eg: `agents.md`)").
				Value(&state.Name).
				Validate(requiredString),
			huh.NewSelect[asset.Type]().
				Key("type").
				Title("type").
				Description("The type of the asset (eg: `agents_doc`)").
				Value(&state.Type).
				Options(assetTypeOptions()...),
			huh.NewText().
				Key("description").
				Title("description").
				Description("Describe the asset").
				Value(&state.Description).
				Validate(requiredString),
			huh.NewInput().
				Key("tags").
				Title("tags").
				Description(`Assign (optional) tags, eg: "git, build"`).
				Value(&state.Tags),
			huh.NewMultiSelect[string]().
				Key("compatible_agents").
				Title("compatible agents").
				Description("Multi-select of agents this asset renders for. Empty means \"all enabled agents\".").
				Value(&state.CompatibleAgents).
				Options(AgentOptions()...),
			huh.NewInput().
				Key("exclusive_group").
				Title("exclusive group").
				Description("Assign (optional) exclusive group (eg: `agents_doc`)").
				Value(&state.ExclusiveGroup),
		),
	)
	return form, state
}

func assetManifestFromState(state *createAssetState) asset.Manifest {
	return asset.Manifest{
		ID:               utils.Slug(state.Name),
		Name:             state.Name,
		Type:             state.Type,
		Description:      state.Description,
		Tags:             parseTags(state.Tags),
		CompatibleAgents: state.CompatibleAgents,
		ExclusiveGroup:   state.ExclusiveGroup,
	}
}

func assetTypeOptions() []huh.Option[asset.Type] {
	types := asset.AllTypes()
	out := make([]huh.Option[asset.Type], 0, len(types))
	for _, t := range types {
		out = append(out, huh.NewOption(string(t), t))
	}
	return out
}

func joinTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	out := tags[0]
	for _, t := range tags[1:] {
		out += ", " + t
	}
	return out
}
