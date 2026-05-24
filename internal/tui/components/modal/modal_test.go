package modal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// fakeContent is a programmable [Content] used to drive Modal.Update in
// isolation. resolutionFn lets each test decide when the content reports a
// terminal state; initCmd and returnCmd are sentinels returned from Init and
// Update respectively, so tests can observe what the modal forwards.
type fakeContent struct {
	view         string
	updates      int
	lastMsg      tea.Msg
	resolutionFn func() (ResolutionState, any)
	initCmd      tea.Cmd
	returnCmd    tea.Cmd
}

func (f *fakeContent) Init() tea.Cmd { return f.initCmd }

func (f *fakeContent) Update(msg tea.Msg) (Content, tea.Cmd) {
	f.updates++
	f.lastMsg = msg
	return f, f.returnCmd
}

func (f *fakeContent) View() string { return f.view }

func (f *fakeContent) Resolution() (ResolutionState, any) {
	if f.resolutionFn == nil {
		return Active, nil
	}
	return f.resolutionFn()
}

func TestModal_IDReturnsConstructorID(t *testing.T) {
	m := New("wizard", &fakeContent{})

	if got := m.ID(); got != "wizard" {
		t.Fatalf("ID() = %q, want %q", got, "wizard")
	}
}

func TestModal_NewPanicsOnNilContent(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil content, got none")
		}
	}()
	New("id", nil)
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
	fake := &fakeContent{} // resolutionFn nil ⇒ Active

	m := New("id", fake)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'a'})

	if cmd != nil {
		t.Fatalf("expected nil cmd while active, got non-nil")
	}
}

func TestModal_EmitsResolvedMsgWhenContentDone(t *testing.T) {
	fake := &fakeContent{
		resolutionFn: func() (ResolutionState, any) { return Confirmed, "payload" },
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
		resolutionFn: func() (ResolutionState, any) { return Cancelled, nil },
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
		resolutionFn: func() (ResolutionState, any) { return Confirmed, nil },
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

func TestModal_BatchesContentCmdWithResolve(t *testing.T) {
	innerMsg := "from-content"
	fake := &fakeContent{
		returnCmd:    func() tea.Msg { return innerMsg },
		resolutionFn: func() (ResolutionState, any) { return Confirmed, "payload" },
	}
	m := New("wizard", fake)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x'})
	if cmd == nil {
		t.Fatalf("expected batched cmd, got nil")
	}

	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", cmd())
	}

	var sawInner, sawResolved bool
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		switch v := sub().(type) {
		case ResolvedMsg:
			if v.ID != "wizard" || !v.Confirmed || v.Value != "payload" {
				t.Errorf("ResolvedMsg = %#v", v)
			}
			sawResolved = true
		case string:
			if v == innerMsg {
				sawInner = true
			}
		}
	}
	if !sawInner {
		t.Errorf("inner content msg %q missing from batch", innerMsg)
	}
	if !sawResolved {
		t.Errorf("ResolvedMsg missing from batch")
	}
}

func TestModal_InitDelegatesToContent(t *testing.T) {
	sentinel := "init-sentinel"
	fake := &fakeContent{initCmd: func() tea.Msg { return sentinel }}
	m := New("id", fake)

	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("Init() returned nil, want fake's initCmd")
	}
	if got := cmd(); got != sentinel {
		t.Errorf("Init()() = %v, want %v", got, sentinel)
	}
}

func TestModal_OptionsApply(t *testing.T) {
	fake := &fakeContent{view: "BODY"}
	customBorder := lipgloss.Border{
		Top: "*", Bottom: "*", Left: "*", Right: "*",
		TopLeft: "*", TopRight: "*", BottomLeft: "*", BottomRight: "*",
	}
	customStyle := lipgloss.NewStyle().Border(customBorder)

	m := New("id", fake, WithStyle(customStyle), WithZ(99))

	if z := m.Layer(60, 20).GetZ(); z != 99 {
		t.Errorf("Layer.Z = %d, want 99 (WithZ not applied)", z)
	}
	if view := m.View(); !strings.Contains(view, "*") {
		t.Errorf("View missing custom border char (WithStyle not applied); got:\n%s", view)
	}
}

func TestModal_LayerClampsToZeroWhenContentExceedsCanvas(t *testing.T) {
	fake := &fakeContent{view: "WIDE-CONTENT"}
	m := New("id", fake)

	layer := m.Layer(2, 2)

	if got := layer.GetX(); got != 0 {
		t.Errorf("Layer.X = %d, want 0 (clamp not applied)", got)
	}
	if got := layer.GetY(); got != 0 {
		t.Errorf("Layer.Y = %d, want 0 (clamp not applied)", got)
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
