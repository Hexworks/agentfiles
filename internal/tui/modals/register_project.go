package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// RegisterProjectInput captures the fields collected by the Register Project
// modal. The screen turns it into a `*project.Manifest` and hands it to
// `app.Service.AddProject`, which slugs the id and enforces ownership.
type RegisterProjectInput struct {
	Name          string
	Path          string
	EnabledAgents []string
}

// NewRegisterProject builds the Register Project modal. Asset selection is
// not collected here — projects are registered with no assets selected; the
// user picks them later from the Select Project Assets Screen.
func NewRegisterProject(initial RegisterProjectInput) *modal.Modal {
	form, state := buildRegisterProject(initial)
	return modal.NewForm("register-project", form, func(*huh.Form) any {
		return &project.Manifest{
			Name:          state.Name,
			Path:          state.Path,
			EnabledAgents: state.EnabledAgents,
		}
	})
}

func buildRegisterProject(initial RegisterProjectInput) (*huh.Form, *RegisterProjectInput) {
	state := &RegisterProjectInput{
		Name:          initial.Name,
		Path:          initial.Path,
		EnabledAgents: append([]string(nil), initial.EnabledAgents...),
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
				Description("Multi-select of agents to enable for this project. At least one required.").
				Value(&state.EnabledAgents).
				Options(AgentOptions()...).
				Validate(requiredAgents),
		),
	)
	return form, state
}
