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

// NewRegisterProfile builds the Register Profile modal.
func NewRegisterProfile(initial RegisterProfileInput) *modal.Modal {
	form, _, extract, _ := buildRegisterProfile(initial)
	return modal.NewForm("register-profile", form, extract, modal.WithCaption("Registering Profile"))
}

func buildRegisterProfile(initial RegisterProfileInput) (*huh.Form, *RegisterProfileInput, func(*huh.Form) any, []huh.Field) {
	state := &RegisterProfileInput{Path: initial.Path}
	fields := []huh.Field{
		pathDisplayNote(&state.Path, "Profile directory picked in the previous step"),
	}
	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any { return *state }, fields
}
