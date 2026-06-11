package modals

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// submitForm flips the form into the same submission state the runtime
// reaches when the user presses Enter on the last field of the last group.
// It is the test-side equivalent of the user completing the form: validate
// every required field, mark all groups submitted, and set State to
// StateCompleted so the modal adapter emits a Confirmed [modal.ResolvedMsg].
//
// We bypass the message pump here because huh's internal nextField /
// nextGroup messages are unexported, which makes a real key-event pump
// fragile. The values written to the bound state pointers — the same
// pointers the field bindings use — are the exact values the form would
// have stored after a real Enter sequence.
func submitForm(t *testing.T, form *huh.Form) {
	t.Helper()
	if errs := form.Errors(); len(errs) > 0 {
		t.Fatalf("form has validation errors before submit: %v", errs)
	}
	form.State = huh.StateCompleted
}

// abortForm mirrors what huh does on the Quit binding (ctrl+c / esc): set
// the form into StateAborted so the modal emits a Cancelled
// [modal.ResolvedMsg] with Confirmed=false.
func abortForm(form *huh.Form) {
	form.State = huh.StateAborted
}

// drainResolvedMsg expects the next message produced by `cmd` to be a
// [modal.ResolvedMsg] (possibly wrapped in a [tea.BatchMsg]) and returns
// it. Mirrors the helper in `internal/tui/components/modal/modal_test.go`.
func drainResolvedMsg(t *testing.T, cmd tea.Cmd) modal.ResolvedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected a tea.Cmd, got nil")
	}
	msg := cmd()
	switch v := msg.(type) {
	case modal.ResolvedMsg:
		return v
	case tea.BatchMsg:
		for _, sub := range v {
			if sub == nil {
				continue
			}
			if r, ok := sub().(modal.ResolvedMsg); ok {
				return r
			}
		}
		t.Fatalf("batch produced no ResolvedMsg: %#v", v)
	}
	t.Fatalf("expected ResolvedMsg, got %T (%v)", msg, msg)
	return modal.ResolvedMsg{}
}

// runResolvedThroughModal builds a modal around the supplied form +
// extract using `modal.NewForm`, then drives `m.Update` once so the modal
// observes the completed lifecycle and emits a [modal.ResolvedMsg]. The
// returned message is the ResolvedMsg the parent screen would see.
func runResolvedThroughModal(t *testing.T, id string, form *huh.Form, extract func(*huh.Form) any) modal.ResolvedMsg {
	t.Helper()
	m := modal.NewForm(id, form, extract)
	_, cmd := m.Update(tea.KeyPressMsg{})
	return drainResolvedMsg(t, cmd)
}
