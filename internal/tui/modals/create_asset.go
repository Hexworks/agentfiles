package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// createAssetState is the in-flight form state shared by the field bindings
// and the extract closure. It mirrors `asset.Manifest` but stores tags as
// the user-facing comma-separated string so the input field can bind to it
// directly.
type createAssetState struct {
	ID               string
	Name             string
	Type             asset.Type
	Description      string
	Tags             string
	CompatibleAgents []string
	ExclusiveGroup   string
}

// NewCreateAsset builds the Create Asset modal. `initial` lets callers preload
// fields (also used by tests). The resulting payload is an `asset.Manifest`;
// when the user leaves `ID` blank, `app.Service.InitAsset` derives it from the
// name via `utils.Slug`, mirroring how `AddProject` slugs project ids.
func NewCreateAsset(initial asset.Manifest) *modal.Modal {
	form, _, extract := buildCreateAsset(initial, asset.AllTypes())
	return modal.NewForm("create-asset", form, extract, modal.WithCaption("Creating Asset"))
}

// NewCreateAssetFromFolder is the Register-as-Asset variant of the Create
// Asset modal. The type select is restricted to asset.FolderRegisterableTypes
// — the convention-based types whose content can come straight from a folder;
// the generic types need explicit projections the folder flow does not
// collect. caption surfaces the folder's file count and size so the user sees
// what will be copied before confirming.
func NewCreateAssetFromFolder(initial asset.Manifest, caption string) *modal.Modal {
	form, _, extract := buildCreateAsset(initial, asset.FolderRegisterableTypes())
	return modal.NewForm("create-asset", form, extract, modal.WithCaption(caption))
}

func buildCreateAsset(initial asset.Manifest, types []asset.Type) (*huh.Form, *createAssetState, func(*huh.Form) any) {
	state := &createAssetState{
		ID:               initial.ID,
		Name:             initial.Name,
		Type:             initial.Type,
		Description:      initial.Description,
		Tags:             JoinTags(initial.Tags),
		CompatibleAgents: config.AgentStrings(initial.CompatibleAgents),
		ExclusiveGroup:   initial.ExclusiveGroup,
	}
	form := huh.NewForm(
		huh.NewGroup(
			idInput(&state.ID, "Identifier (leave blank to derive from name)"),
			nameInput(&state.Name, "The name of the asset (eg: `agents.md`)"),
			assetTypeSelect(&state.Type, "The type of the asset (eg: `agents_doc`)", types),
			descriptionText(&state.Description, "Describe the asset"),
		),
		huh.NewGroup(
			tagsInput(&state.Tags, `Assign (optional) tags, eg: "git, build"`),
			compatibleAgentsSelect(&state.CompatibleAgents, "Multi-select of agents this asset renders for. Empty means \"all enabled agents\"."),
			exclusiveGroupInput(&state.ExclusiveGroup, "Assign (optional) exclusive group (eg: `agents_doc`)"),
		),
	).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any { return assetManifestFromState(state) }
}

func assetManifestFromState(state *createAssetState) asset.Manifest {
	return asset.Manifest{
		ID:               state.ID,
		Name:             state.Name,
		Type:             state.Type,
		Description:      state.Description,
		Tags:             ParseTags(state.Tags),
		CompatibleAgents: config.ToAgents(state.CompatibleAgents),
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
