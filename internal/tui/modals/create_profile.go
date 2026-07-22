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
	form, _, extract, _ := buildCreateProfile(initial)
	return modal.NewForm("create-profile", form, extract, modal.WithCaption("Creating Profile"))
}

func buildCreateProfile(initial CreateProfileInput) (*huh.Form, *CreateProfileInput, func(*huh.Form) any, []huh.Field) {
	state := &CreateProfileInput{Name: initial.Name, Path: initial.Path}
	fields := []huh.Field{
		nameInput(&state.Name, "Display name for the profile"),
		pathDisplayNote(&state.Path, "Profile directory picked in the previous step"),
	}
	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any { return *state }, fields
}
