package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// EditProjectInput is the typed payload delivered through `modal.ResolvedMsg`
// when the Edit Project form is submitted (and the prefill accepted on the way
// in). It carries only the editable fields; the screen that opens the modal
// owns the underlying `*project.Manifest` and is responsible for merging these
// values back in while preserving `ID`, `SelectedAssetIDs`, and `CreatedAt`.
//
// EnabledAgents is typed `[]agent.Agent`: the string↔typed conversion happens
// inside this modal (at the huh boundary), matching the Register Project modal
// so both project modals convert at the same seam.
type EditProjectInput struct {
	Name          string
	Path          string
	EnabledAgents []agent.Agent
}

// editProjectState holds the raw huh form bindings. EnabledAgents stays
// `[]string` because huh.MultiSelect binds strings; buildEditProject converts
// to and from the typed EditProjectInput at the modal boundary.
type editProjectState struct {
	Name          string
	Path          string
	EnabledAgents []string
}

// NewEditProject builds the Edit Project modal prefilled from existing
// values. `Project.ID` and `SelectedAssetIDs` are not exposed because the
// modal cannot edit them — see `EditProjectInput`.
func NewEditProject(initial EditProjectInput) *modal.Modal {
	form, _, extract := buildEditProject(initial)
	return modal.NewForm("edit-project", form, extract, modal.WithCaption("Editing Project"))
}

func buildEditProject(initial EditProjectInput) (*huh.Form, *editProjectState, func(*huh.Form) any) {
	state := &editProjectState{
		Name:          initial.Name,
		Path:          initial.Path,
		EnabledAgents: agent.Strings(initial.EnabledAgents),
	}
	form := huh.NewForm(
		huh.NewGroup(
			nameInput(&state.Name, "The name of the project"),
			pathInput(&state.Path, "The path of the project"),
			enabledAgentsSelect(&state.EnabledAgents, "Multi-select of agents enabled for this project. At least one required."),
		),
	).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any {
		return EditProjectInput{
			Name:          state.Name,
			Path:          state.Path,
			EnabledAgents: agent.FromStrings(state.EnabledAgents),
		}
	}
}
