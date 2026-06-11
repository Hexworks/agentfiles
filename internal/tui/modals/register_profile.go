package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// RegisterProfileInput is the typed payload returned by the Register Profile
// modal.
type RegisterProfileInput struct {
	Path string
}

// NewRegisterProfile builds the Register Profile modal.
func NewRegisterProfile(initial RegisterProfileInput) *modal.Modal {
	form, state := buildRegisterProfile(initial)
	return modal.NewForm("register-profile", form, func(*huh.Form) any {
		return *state
	})
}

func buildRegisterProfile(initial RegisterProfileInput) (*huh.Form, *RegisterProfileInput) {
	state := &RegisterProfileInput{Path: initial.Path}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("path").
				Title("Path").
				Description("Profile directory path. ~ is expanded.").
				Value(&state.Path).
				Validate(requiredString),
		),
	)
	return form, state
}
