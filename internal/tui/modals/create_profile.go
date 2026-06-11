package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// CreateProfileInput is the typed payload delivered through
// `modal.ResolvedMsg.Value` when the Create Profile form is submitted.
type CreateProfileInput struct {
	Name string
	Path string
}

// NewCreateProfile builds the Create Profile modal. `initial` lets callers
// preload the input fields (useful for testing and for "retry after error"
// flows); a zero value starts the form empty.
func NewCreateProfile(initial CreateProfileInput) *modal.Modal {
	form, state := buildCreateProfile(initial)
	return modal.NewForm("create-profile", form, func(*huh.Form) any {
		return *state
	})
}

// buildCreateProfile assembles the form and the shared state pointer the
// fields write into. Exposed at package scope so unit tests can drive the
// form without going through the modal wrapper.
func buildCreateProfile(initial CreateProfileInput) (*huh.Form, *CreateProfileInput) {
	state := &CreateProfileInput{Name: initial.Name, Path: initial.Path}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Name").
				Description("Display name for the profile").
				Value(&state.Name).
				Validate(requiredString),
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
