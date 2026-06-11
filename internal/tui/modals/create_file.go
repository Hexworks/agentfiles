package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// CreateFileInput is the typed payload returned by the Create File modal.
// `Path` is the file path relative to the asset folder.
type CreateFileInput struct {
	Path string
}

// NewCreateFile builds the Create File modal.
func NewCreateFile(initial CreateFileInput) *modal.Modal {
	form, state := buildCreateFile(initial)
	return modal.NewForm("create-file", form, func(*huh.Form) any {
		return *state
	})
}

func buildCreateFile(initial CreateFileInput) (*huh.Form, *CreateFileInput) {
	state := &CreateFileInput{Path: initial.Path}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("path").
				Title("Path").
				Description("File path relative to the asset folder").
				Value(&state.Path).
				Validate(requiredString),
		),
	)
	return form, state
}
