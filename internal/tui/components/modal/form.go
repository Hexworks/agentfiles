package modal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// formContent adapts a [huh.Form] to the [Content] interface. The form's
// own State drives the modal's resolution: StateCompleted -> Confirmed,
// StateAborted -> Cancelled. On Confirmed, extract is called to convert the
// form into a caller-defined payload, so *huh.Form does not leak out of this
// package.
type formContent struct {
	form    *huh.Form
	extract func(*huh.Form) any
}

func (f *formContent) Init() tea.Cmd { return f.form.Init() }

func (f *formContent) Update(msg tea.Msg) (Content, tea.Cmd) {
	model, cmd := f.form.Update(msg)
	if updated, ok := model.(*huh.Form); ok {
		f.form = updated
	}
	return f, cmd
}

func (f *formContent) View() string { return f.form.View() }

func (f *formContent) Resolution() (ResolutionState, any) {
	switch f.form.State {
	case huh.StateCompleted:
		return Confirmed, f.extract(f.form)
	case huh.StateAborted:
		return Cancelled, nil
	default:
		return Active, nil
	}
}

// NewForm constructs a Modal that hosts a huh form. extract is called once,
// on successful completion, to convert the form into the payload delivered
// through [ResolvedMsg].Value — define a typed result struct in the calling
// package and read fields off the form inside extract, so *huh.Form does not
// leave this package.
//
// extract must be non-nil; the simplest implementation that preserves the
// previous "value is the form itself" behavior is
// func(f *huh.Form) any { return f }.
func NewForm(id string, form *huh.Form, extract func(*huh.Form) any, opts ...Option) *Modal {
	if extract == nil {
		panic("modal: nil extract")
	}
	return New(id, &formContent{form: form, extract: extract}, opts...)
}
