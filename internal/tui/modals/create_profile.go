package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
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
	form, _, extract := buildCreateProfile(initial)
	return modal.NewForm("create-profile", form, extract)
}

func buildCreateProfile(initial CreateProfileInput) (*huh.Form, *CreateProfileInput, func(*huh.Form) any) {
	state := &CreateProfileInput{Name: initial.Name, Path: initial.Path}
	form := huh.NewForm(
		huh.NewGroup(
			nameInput(&state.Name, "Display name for the profile"),
			pathInput(&state.Path, "Profile directory path. ~ is expanded."),
		),
	).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any { return *state }
}
