package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// RegisterProfileInput is the typed payload returned by the Register Profile
// modal.
type RegisterProfileInput struct {
	Path string
}

// builtRegisterProfileForm is the internal handle produced by
// buildRegisterProfile. Production wraps `.Form` + `.Extract`; tests reach
// into `.State` and `.Fields` to inspect field types without reflecting
// into huh internals.
type builtRegisterProfileForm struct {
	Form    *huh.Form
	State   *RegisterProfileInput
	Extract func(*huh.Form) any
	Fields  []huh.Field
}

// NewRegisterProfile builds the Register Profile modal.
func NewRegisterProfile(initial RegisterProfileInput) *modal.Modal {
	built := buildRegisterProfile(initial)
	return modal.NewForm("register-profile", built.Form, built.Extract, modal.WithCaption("Registering Profile"))
}

func buildRegisterProfile(initial RegisterProfileInput) builtRegisterProfileForm {
	state := &RegisterProfileInput{Path: initial.Path}
	fields := []huh.Field{
		pathDisplayNote(state.Path, "Profile directory picked in the previous step"),
	}
	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(styles.HuhTheme())
	return builtRegisterProfileForm{
		Form:    form,
		State:   state,
		Extract: func(*huh.Form) any { return *state },
		Fields:  fields,
	}
}
