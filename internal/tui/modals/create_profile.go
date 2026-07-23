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

// builtCreateProfileForm is the internal handle produced by
// buildCreateProfile. See [builtRegisterProfileForm] for the rationale.
type builtCreateProfileForm struct {
	Form    *huh.Form
	State   *CreateProfileInput
	Extract func(*huh.Form) any
	Fields  []huh.Field
}

// NewCreateProfile builds the Create Profile modal. `initial` lets callers
// preload the input fields (useful for testing and for "retry after error"
// flows); a zero value starts the form empty.
func NewCreateProfile(initial CreateProfileInput) *modal.Modal {
	built := buildCreateProfile(initial)
	return modal.NewForm("create-profile", built.Form, built.Extract, modal.WithCaption("Creating Profile"))
}

func buildCreateProfile(initial CreateProfileInput) builtCreateProfileForm {
	state := &CreateProfileInput{Name: initial.Name, Path: initial.Path}
	fields := []huh.Field{
		nameInput(&state.Name, "Display name for the profile"),
		pathDisplayNote(state.Path, "Profile directory picked in the previous step"),
	}
	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(styles.HuhTheme())
	return builtCreateProfileForm{
		Form:    form,
		State:   state,
		Extract: func(*huh.Form) any { return *state },
		Fields:  fields,
	}
}
