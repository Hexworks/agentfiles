package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// editProjectState is the in-flight form state shared by the field
// bindings and the extract closure.
type editProjectState struct {
	Name          string
	Path          string
	EnabledAgents []string
}

// NewEditProject builds the Edit Project modal prefilled from an existing
// project manifest. `Project.ID` and `SelectedAssetIDs` are not exposed —
// the screen keeps them untouched. The payload is the modified
// `*project.Manifest`.
func NewEditProject(existing *project.Manifest) *modal.Modal {
	form, state := buildEditProject(existing)
	return modal.NewForm("edit-project", form, func(*huh.Form) any {
		updated := *existing
		updated.Name = state.Name
		updated.Path = state.Path
		updated.EnabledAgents = state.EnabledAgents
		return &updated
	})
}

func buildEditProject(existing *project.Manifest) (*huh.Form, *editProjectState) {
	state := &editProjectState{
		Name:          existing.Name,
		Path:          existing.Path,
		EnabledAgents: append([]string(nil), existing.EnabledAgents...),
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("name").
				Description("The name of the project").
				Value(&state.Name).
				Validate(requiredString),
			huh.NewInput().
				Key("path").
				Title("path").
				Description("The path of the project").
				Value(&state.Path).
				Validate(requiredString),
			huh.NewMultiSelect[string]().
				Key("enabled_agents").
				Title("enabled agents").
				Description("Multi-select of agents enabled for this project. At least one required.").
				Value(&state.EnabledAgents).
				Options(AgentOptions()...).
				Validate(requiredAgents),
		),
	)
	return form, state
}
