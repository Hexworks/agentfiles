package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// RegisterProjectInput captures the fields collected by the Register Project
// modal. The screen turns it into a `*project.Manifest` and hands it to
// `app.Service.AddProject`, which enforces ownership.
type RegisterProjectInput struct {
	Name          string
	Path          string
	EnabledAgents []string
}

// NewRegisterProject builds the Register Project modal. Asset selection is
// not collected here — projects are registered with no assets selected; the
// user picks them later from the Select Project Assets Screen. The resulting
// payload is a `*project.Manifest` constructed via `project.NewDraft`, so the
// returned value passes `Manifest.Validate()` without further work.
func NewRegisterProject(initial RegisterProjectInput) *modal.Modal {
	form, _, extract := buildRegisterProject(initial)
	return modal.NewForm("register-project", form, extract)
}

func buildRegisterProject(initial RegisterProjectInput) (*huh.Form, *RegisterProjectInput, func(*huh.Form) any) {
	state := &RegisterProjectInput{
		Name:          initial.Name,
		Path:          initial.Path,
		EnabledAgents: append([]string(nil), initial.EnabledAgents...),
	}
	form := huh.NewForm(
		huh.NewGroup(
			nameInput(&state.Name, "The name of the project"),
			pathInput(&state.Path, "The path of the project"),
			enabledAgentsSelect(&state.EnabledAgents, "Multi-select of agents to enable for this project. At least one required."),
		),
	)
	return form, state, func(*huh.Form) any {
		return project.NewDraft(state.Name, state.Path, state.EnabledAgents)
	}
}
