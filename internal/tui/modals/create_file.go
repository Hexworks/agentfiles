package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// CreateFileInput is the typed payload returned by the Create File modal.
// `Path` is the file path relative to the asset folder.
type CreateFileInput struct {
	Path string
}

// NewCreateFile builds the Create File modal.
func NewCreateFile(initial CreateFileInput) *modal.Modal {
	form, _, extract := buildCreateFile(initial)
	return modal.NewForm("create-file", form, extract)
}

func buildCreateFile(initial CreateFileInput) (*huh.Form, *CreateFileInput, func(*huh.Form) any) {
	state := &CreateFileInput{Path: initial.Path}
	form := huh.NewForm(
		huh.NewGroup(
			pathInput(&state.Path, "File path relative to the asset folder"),
		),
	).WithTheme(styles.HuhTheme())
	return form, state, func(*huh.Form) any { return *state }
}
