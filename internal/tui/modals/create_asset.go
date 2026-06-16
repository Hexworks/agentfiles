package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
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

// NewCreateAsset builds the Create Asset modal. `initial` lets callers preload
// fields (also used by tests). The resulting payload is an `asset.Manifest`
// with `ID` left empty — `app.Service.InitAsset` derives the id from the name
// via `utils.Slug`, mirroring how `AddProject` slugs project ids.
func NewCreateAsset(initial asset.Manifest) *modal.Modal {
	form, _, extract := buildCreateAsset(initial)
	return modal.NewForm("create-asset", form, extract)
}

func buildCreateAsset(initial asset.Manifest) (*huh.Form, *createAssetState, func(*huh.Form) any) {
	state := &createAssetState{
		Name:             initial.Name,
		Type:             initial.Type,
		Description:      initial.Description,
		Tags:             JoinTags(initial.Tags),
		CompatibleAgents: append([]string(nil), initial.CompatibleAgents...),
		ExclusiveGroup:   initial.ExclusiveGroup,
	}
	form := huh.NewForm(
		huh.NewGroup(
			nameInput(&state.Name, "The name of the asset (eg: `agents.md`)"),
			assetTypeSelect(&state.Type, "The type of the asset (eg: `agents_doc`)"),
			descriptionText(&state.Description, "Describe the asset"),
			tagsInput(&state.Tags, `Assign (optional) tags, eg: "git, build"`),
			compatibleAgentsSelect(&state.CompatibleAgents, "Multi-select of agents this asset renders for. Empty means \"all enabled agents\"."),
			exclusiveGroupInput(&state.ExclusiveGroup, "Assign (optional) exclusive group (eg: `agents_doc`)"),
		),
	).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any { return assetManifestFromState(state) }
}

func assetManifestFromState(state *createAssetState) asset.Manifest {
	return asset.Manifest{
		Name:             state.Name,
		Type:             state.Type,
		Description:      state.Description,
		Tags:             ParseTags(state.Tags),
		CompatibleAgents: state.CompatibleAgents,
		ExclusiveGroup:   state.ExclusiveGroup,
	}
}

// JoinTags renders a tag slice as the comma-separated string the modal
// form binds to.
func JoinTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	out := tags[0]
	for _, t := range tags[1:] {
		out += ", " + t
	}
	return out
}
