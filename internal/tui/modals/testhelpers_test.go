package modals

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// cmdTimeout caps how long the pump waits for a single command to
// produce its message. Most relevant huh commands (blur, nextGroup,
// nextField) return synchronously, but textinput focus emits a
// cursor.Blink command that blocks on a context deadline (~530ms).
// Skipping those keeps the pump fast without dropping the messages we
// actually need (nextGroupMsg, validation flags).
const cmdTimeout = 5 * time.Millisecond

// submitForm drives the form to terminal completion by repeatedly
// invoking huh's exported NextField() pump and draining the returned
// commands. nextFieldMsg makes the focused field blur (running its
// Validate(...) callback), the group's nextField cycles focus, and once
// the last field has been blurred huh dispatches a nextGroup command
// which — when drained — completes the form. With prefilled state
// pointers the validators see populated values and pass, so the pump
// terminates within a handful of iterations.
//
// Unlike a direct `form.State = huh.StateCompleted` flip this helper
// actually exercises Validate(...) callbacks and the field-to-state
// pointer wiring. Tests that build a form with empty required fields
// can use expectFormStuck to assert the pump never reaches
// StateCompleted.
func submitForm(t *testing.T, form *huh.Form) {
	t.Helper()
	// Init runs synchronously: it sets the first group active and focuses
	// the first field. The returned Sequence cmd carries only cosmetic
	// updates (cursor blink, title hydration, window size request); we
	// do not need to drain it for state transitions.
	form.Init()
	for i := 0; form.State == huh.StateNormal; i++ {
		if i > 50 {
			t.Fatalf("form did not complete after 50 NextField iterations; state=%v errors=%v",
				form.State, form.Errors())
		}
		drainCmd(form, form.NextField())
	}
	if form.State != huh.StateCompleted {
		t.Fatalf("form terminated in unexpected state %v (errors=%v)", form.State, form.Errors())
	}
}

// expectFormStuck drives the form for the safety budget and asserts it
// never reaches a terminal state — used by required-field rejection
// tests. A validator that blocks a required field keeps group.Errors()
// non-empty, so nextGroup short-circuits and form.State stays Normal.
func expectFormStuck(t *testing.T, form *huh.Form) {
	t.Helper()
	form.Init()
	for i := 0; i < 30; i++ {
		if form.State != huh.StateNormal {
			t.Fatalf("form transitioned to %v at iter %d; validator should have blocked it (errors=%v)",
				form.State, i, form.Errors())
		}
		drainCmd(form, form.NextField())
	}
	if len(form.Errors()) == 0 {
		t.Fatalf("form remained Normal but reported no validation errors; expected at least one")
	}
}

// drainCmd repeatedly executes cmd, feeds resulting messages back into
// form.Update, and recurses into BatchMsgs. Each cmd call is bounded by
// cmdTimeout so blocking commands (cursor blink) do not stall the pump
// — they are skipped, which is fine because they only affect cosmetics.
// The commands we actually need to land (nextGroup, blur side effects)
// return synchronously well within the timeout.
func drainCmd(form *huh.Form, cmd tea.Cmd) {
	for cmd != nil {
		msg, ok := runCmd(cmd)
		if !ok || msg == nil {
			return
		}
		if batch, isBatch := msg.(tea.BatchMsg); isBatch {
			for _, sub := range batch {
				drainCmd(form, sub)
			}
			return
		}
		_, cmd = form.Update(msg)
	}
}

func runCmd(cmd tea.Cmd) (tea.Msg, bool) {
	ch := make(chan tea.Msg, 1)
	go func() {
		defer func() { _ = recover() }()
		ch <- cmd()
	}()
	select {
	case m := <-ch:
		return m, true
	case <-time.After(cmdTimeout):
		return nil, false
	}
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
