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
	form, _, extract := buildRegisterProfile(initial)
	return modal.NewForm("register-profile", form, extract)
}

func buildRegisterProfile(initial RegisterProfileInput) (*huh.Form, *RegisterProfileInput, func(*huh.Form) any) {
	state := &RegisterProfileInput{Path: initial.Path}
	form := huh.NewForm(
		huh.NewGroup(
			pathInput(&state.Path, "Profile directory path. ~ is expanded."),
		),
	)
	return form, state, func(*huh.Form) any { return *state }
}
