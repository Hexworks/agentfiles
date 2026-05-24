package modal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// fakeContent is a programmable [Content] used to drive Modal.Update in
// isolation. doneFn lets each test decide when the content reports done.
type fakeContent struct {
	view      string
	updates   int
	lastMsg   tea.Msg
	doneFn    func() (bool, bool, any)
	returnCmd tea.Cmd
}

func (f *fakeContent) Init() tea.Cmd { return nil }

func (f *fakeContent) Update(msg tea.Msg) (Content, tea.Cmd) {
	f.updates++
	f.lastMsg = msg
	return f, f.returnCmd
}

func (f *fakeContent) View() string { return f.view }

func (f *fakeContent) Done() (bool, bool, any) {
	if f.doneFn == nil {
		return false, false, nil
	}
	return f.doneFn()
}

func TestModal_IDReturnsConstructorID(t *testing.T) {
	m := New("wizard", &fakeContent{})

	if got := m.ID(); got != "wizard" {
		t.Fatalf("ID() = %q, want %q", got, "wizard")
	}
}

func TestModal_UpdateForwardsToContent(t *testing.T) {
	fake := &fakeContent{}
	m := New("id", fake)

	msg := tea.KeyPressMsg{Code: 'x'}
	_, _ = m.Update(msg)

	if fake.updates != 1 {
		t.Fatalf("content.Update called %d times, want 1", fake.updates)
	}
	if fake.lastMsg != msg {
		t.Fatalf("content received %v, want %v", fake.lastMsg, msg)
	}
}

func TestModal_NoResolvedMsgWhileActive(t *testing.T) {
	fake := &fakeContent{} // doneFn nil ⇒ not done

	m := New("id", fake)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'a'})

	if cmd != nil {
		t.Fatalf("expected nil cmd while active, got %T", cmd())
	}
}

func TestModal_EmitsResolvedMsgWhenContentDone(t *testing.T) {
	fake := &fakeContent{
		doneFn: func() (bool, bool, any) { return true, true, "payload" },
	}
	m := New("wizard", fake)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})

	got := drainResolved(t, cmd)
	if got.ID != "wizard" {
		t.Errorf("ID = %q, want %q", got.ID, "wizard")
	}
	if !got.Confirmed {
		t.Errorf("Confirmed = false, want true")
	}
	if got.Value != "payload" {
		t.Errorf("Value = %v, want %q", got.Value, "payload")
	}
}

func TestModal_CancelMapsToConfirmedFalse(t *testing.T) {
	fake := &fakeContent{
		doneFn: func() (bool, bool, any) { return true, false, nil },
	}
	m := New("wizard", fake)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 27}) // esc

	got := drainResolved(t, cmd)
	if got.Confirmed {
		t.Errorf("Confirmed = true, want false on cancel")
	}
	if got.Value != nil {
		t.Errorf("Value = %v, want nil on cancel", got.Value)
	}
}

func TestModal_UpdateIsInertAfterResolution(t *testing.T) {
	fake := &fakeContent{
		doneFn: func() (bool, bool, any) { return true, true, nil },
	}
	m := New("id", fake)

	// First update resolves the modal.
	_, _ = m.Update(tea.KeyPressMsg{Code: 'a'})
	updatesAfterResolve := fake.updates

	// Subsequent updates must not reach the content and must return nil cmd.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'b'})
	if cmd != nil {
		t.Errorf("expected nil cmd after resolution, got non-nil")
	}
	if fake.updates != updatesAfterResolve {
		t.Errorf("content.Update was called %d times after resolution; want no change from %d",
			fake.updates, updatesAfterResolve)
	}
}

func TestModal_RenderCompositesOverBackground(t *testing.T) {
	fake := &fakeContent{view: "MODAL-BODY"}
	m := New("id", fake)

	const bg = "BACKGROUND-TEXT-XYZ"
	out := ansi.Strip(m.Render(bg, 60, 20))

	if !strings.Contains(out, "MODAL-BODY") {
		t.Errorf("rendered output missing modal content; got:\n%s", out)
	}
	if !strings.Contains(out, "BACKGROUND-TEXT-XYZ") {
		t.Errorf("rendered output missing background; got:\n%s", out)
	}
}

// drainResolved executes cmd, asserts it produced a ResolvedMsg, and returns
// it. Fails the test immediately on any mismatch so callers can use the
// return value without further nil-checks.
func drainResolved(t *testing.T, cmd tea.Cmd) ResolvedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected a tea.Cmd, got nil")
	}
	msg := cmd()
	// When the inner content also returned a command, Modal.Update batches
	// them. Walk a batch once to find the ResolvedMsg.
	switch v := msg.(type) {
	case ResolvedMsg:
		return v
	case tea.BatchMsg:
		for _, sub := range v {
			if sub == nil {
				continue
			}
			if r, ok := sub().(ResolvedMsg); ok {
				return r
			}
		}
		t.Fatalf("batch produced no ResolvedMsg: %#v", v)
	}
	t.Fatalf("expected ResolvedMsg, got %T (%v)", msg, msg)
	return ResolvedMsg{}
}
