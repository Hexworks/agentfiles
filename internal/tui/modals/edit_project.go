package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// EditProjectInput is the typed payload delivered through `modal.ResolvedMsg`
// when the Edit Project form is submitted. It carries only the editable
// fields; the screen that opens the modal owns the underlying
// `*project.Manifest` and is responsible for merging these values back in
// while preserving `ID`, `SelectedAssetIDs`, and `CreatedAt`.
type EditProjectInput struct {
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

func buildEditProject(initial EditProjectInput) (*huh.Form, *EditProjectInput, func(*huh.Form) any) {
	state := &EditProjectInput{
		Name:          initial.Name,
		Path:          initial.Path,
		EnabledAgents: append([]string(nil), initial.EnabledAgents...),
	}
	form := huh.NewForm(
		huh.NewGroup(
			nameInput(&state.Name, "The name of the project"),
			pathInput(&state.Path, "The path of the project"),
			enabledAgentsSelect(&state.EnabledAgents, "Multi-select of agents enabled for this project. At least one required."),
		),
	).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any { return *state }
}
