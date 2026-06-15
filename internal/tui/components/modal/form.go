package modal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// formContent adapts a [huh.Form] to the [Content] interface. The form's
// own State drives the modal's lifecycle: StateCompleted -> Confirmed,
// StateAborted -> Cancelled. On Confirmed, extract is called to convert the
// form into a caller-defined payload, so *huh.Form does not leak out of this
// package.
type formContent struct {
	form    *huh.Form
	extract func(*huh.Form) any
}

func (f *formContent) Init() tea.Cmd { return f.form.Init() }

func (f *formContent) Update(msg tea.Msg) (Content, tea.Cmd) {
	// Escape aborts every form modal uniformly. huh's default keymap
	// only treats ctrl+c as abort, but users expect Esc to dismiss a
	// dialog — intercept here so every form modal inherits the same
	// behavior without each caller wiring its own keymap override.
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.Code == tea.KeyEsc {
		f.form.State = huh.StateAborted
		return f, nil
	}
	// Swallow WindowSizeMsg so the hosted huh form keeps the compact
	// natural size it had on open. Without this, the first resize event
	// after open (often triggered by huh.Form.Init itself) expands the
	// form to the full viewport width, producing a visible "snap"
	// moments after the modal appears.
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		return f, nil
	}
	model, cmd := f.form.Update(msg)
	if updated, ok := model.(*huh.Form); ok {
		f.form = updated
	}
	return f, cmd
}

func (f *formContent) View() string { return f.form.View() }

func (f *formContent) Lifecycle() (LifecycleState, any) {
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
