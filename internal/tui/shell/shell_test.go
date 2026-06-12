package shell

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

func newTestShell(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	svc := app.New(dir + "/registry.json")
	return New(actions.New(svc), notifications.NewLog())
}

func TestNew_PanicsOnNilActions(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil actions, got none")
		}
	}()
	New(nil, notifications.NewLog())
}

func TestNew_PanicsOnNilLog(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil log, got none")
		}
	}()
	dir := t.TempDir()
	svc := app.New(dir + "/registry.json")
	New(actions.New(svc), nil)
}

func TestUpdate_PushScreenMsgGrowsStack(t *testing.T) {
	m := newTestShell(t)
	if len(m.stack) != 1 {
		t.Fatalf("initial stack depth = %d, want 1", len(m.stack))
	}
	if _, ok := m.stack[0].(*welcomeScreen); !ok {
		t.Fatalf("initial top = %T, want *welcomeScreen", m.stack[0])
	}

	tm, _ := m.Update(PushScreenMsg{Screen: newSettingsScreen()})
	m = tm.(Model)
	if len(m.stack) != 2 {
		t.Fatalf("after push depth = %d, want 2", len(m.stack))
	}
	if _, ok := m.stack[1].(*settingsScreen); !ok {
		t.Fatalf("top after push = %T, want *settingsScreen", m.stack[1])
	}
}

// initSentinelScreen records a known cmd from Init so the shell test
// can verify PushScreenMsg actually runs it.
type initSentinelScreen struct {
	sentinel tea.Msg
}

func (s *initSentinelScreen) Init() tea.Cmd {
	return func() tea.Msg { return s.sentinel }
}
func (s *initSentinelScreen) Update(_ tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (s *initSentinelScreen) Body(_ int) string                  { return "" }
func (s *initSentinelScreen) Title() string                      { return "init-sentinel" }
func (s *initSentinelScreen) StatusKeys() []key.Binding          { return nil }
func (s *initSentinelScreen) InputFocused() bool                 { return false }

type initSentinelMsg struct{}

func TestUpdate_PushScreenMsgRunsNewScreenInit(t *testing.T) {
	m := newTestShell(t)

	want := initSentinelMsg{}
	push := &initSentinelScreen{sentinel: want}

	_, cmd := m.Update(PushScreenMsg{Screen: push})
	if cmd == nil {
		t.Fatalf("PushScreenMsg returned nil cmd, want screen.Init()")
	}
	got := cmd()
	if got != want {
		t.Fatalf("Init cmd produced %v, want %v", got, want)
	}
}

func TestUpdate_PushScreenMsgPropagatesStoredSize(t *testing.T) {
	m := newTestShell(t)

	// Seed the shell with a window size before the push so the new
	// screen has a target to receive.
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = tm.(Model)

	spy := &sizingSpy{}
	tm, cmd := m.Update(PushScreenMsg{Screen: spy})
	m = tm.(Model)
	if cmd == nil {
		t.Fatalf("push with size produced nil cmd")
	}

	// The push batches Init + a synthetic WindowSizeMsg cmd. Drain both
	// and pump any WindowSizeMsg back through the shell so it reaches
	// the active screen via the standard broadcast path.
	collect(t, cmd, func(msg tea.Msg) {
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			tm, _ = m.Update(ws)
			m = tm.(Model)
		}
	})

	if len(spy.received) == 0 {
		t.Fatalf("pushed screen received no WindowSizeMsg, want one")
	}
	got := spy.received[len(spy.received)-1]
	if got.Width != 120 || got.Height != 40 {
		t.Errorf("propagated size = %v, want {120,40}", got)
	}
}

// collect drains cmd into visit for every concrete message it produces,
// recursing into tea.BatchMsg wrappers. Mirrors the helper in
// profiles_test.go but kept local so this file does not import it.
func collect(t *testing.T, cmd tea.Cmd, visit func(tea.Msg)) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			collect(t, c, visit)
		}
		return
	}
	visit(msg)
}

func TestUpdate_PushScreenMsgDedupSameType(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(PushScreenMsg{Screen: newSettingsScreen()})
	m = tm.(Model)
	if len(m.stack) != 2 {
		t.Fatalf("after first push depth = %d, want 2", len(m.stack))
	}

	tm, _ = m.Update(PushScreenMsg{Screen: newSettingsScreen()})
	m = tm.(Model)
	if len(m.stack) != 2 {
		t.Fatalf("after duplicate-type push depth = %d, want 2 (dedup)", len(m.stack))
	}
}

func TestUpdate_PopScreenMsgShrinksStack(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(PushScreenMsg{Screen: newSettingsScreen()})
	m = tm.(Model)

	tm, _ = m.Update(PopScreenMsg{})
	m = tm.(Model)
	if len(m.stack) != 1 {
		t.Fatalf("after pop depth = %d, want 1", len(m.stack))
	}
	if _, ok := m.stack[0].(*welcomeScreen); !ok {
		t.Fatalf("top after pop = %T, want *welcomeScreen", m.stack[0])
	}
}

func TestUpdate_PopScreenMsgZeroesSlot(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(PushScreenMsg{Screen: newSettingsScreen()})
	m = tm.(Model)

	// Reach into the backing array's index-1 slot via re-slice.
	backing := m.stack[:cap(m.stack)]
	if backing[1] == nil {
		t.Fatalf("setup: slot[1] already nil before pop")
	}

	tm, _ = m.Update(PopScreenMsg{})
	m = tm.(Model)

	backing = m.stack[:cap(m.stack)]
	if len(backing) < 2 {
		t.Skip("backing array reallocated; cannot verify slot zeroing")
	}
	if backing[1] != nil {
		t.Errorf("popped slot still references %T, want nil", backing[1])
	}
}

func TestUpdate_PopScreenMsgRootStays(t *testing.T) {
	m := newTestShell(t)

	tm, _ := m.Update(PopScreenMsg{})
	m = tm.(Model)
	if len(m.stack) != 1 {
		t.Fatalf("depth after pop on root = %d, want 1", len(m.stack))
	}
}

// recordingScreen captures every message it sees so tests can assert
// global-key precedence (the screen must NOT see globally intercepted
// keys) and default-routing fan-out.
type recordingScreen struct {
	received []tea.Msg
}

func (s *recordingScreen) Init() tea.Cmd { return nil }
func (s *recordingScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}
func (s *recordingScreen) Body(_ int) string         { return "" }
func (s *recordingScreen) Title() string             { return "rec" }
func (s *recordingScreen) StatusKeys() []key.Binding { return nil }
func (s *recordingScreen) InputFocused() bool        { return false }

func TestUpdate_GlobalKeysInterceptedBeforeScreen(t *testing.T) {
	cases := []struct {
		name    string
		msg     tea.KeyPressMsg
		wantCmd string // "push:<title>" or "quit"
	}{
		{"n notifications", tea.KeyPressMsg{Code: 'n', Text: "n"}, "push:Notifications"},
		{"s settings", tea.KeyPressMsg{Code: 's', Text: "s"}, "push:Settings"},
		{"? help", tea.KeyPressMsg{Code: '?', Text: "?"}, "push:Help"},
		{"q quit", tea.KeyPressMsg{Code: 'q', Text: "q"}, "quit"},
		{"ctrl+c quit", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "quit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestShell(t)
			rec := &recordingScreen{}
			m.stack[0] = rec

			_, cmd := m.Update(tc.msg)
			if cmd == nil {
				t.Fatalf("expected non-nil cmd from global handler")
			}
			if len(rec.received) != 0 {
				t.Fatalf("active screen received intercepted key %v", rec.received)
			}

			out := cmd()
			switch tc.wantCmd {
			case "quit":
				if _, ok := out.(tea.QuitMsg); !ok {
					t.Fatalf("cmd produced %T, want tea.QuitMsg", out)
				}
			default:
				push, ok := out.(PushScreenMsg)
				if !ok {
					t.Fatalf("cmd produced %T, want PushScreenMsg", out)
				}
				if got := push.Screen.Title(); got != strings.TrimPrefix(tc.wantCmd, "push:") {
					t.Fatalf("pushed screen title = %q, want %q", got, strings.TrimPrefix(tc.wantCmd, "push:"))
				}
			}
		})
	}
}

func TestUpdate_NonGlobalKeyReachesScreen(t *testing.T) {
	m := newTestShell(t)
	rec := &recordingScreen{}
	m.stack[0] = rec

	letter := tea.KeyPressMsg{Code: 'x', Text: "x"}
	_, _ = m.Update(letter)

	if len(rec.received) != 1 {
		t.Fatalf("screen received %d msgs, want 1", len(rec.received))
	}
	if rec.received[0] != letter {
		t.Fatalf("screen received %v, want %v", rec.received[0], letter)
	}
}

// customDefaultMsg is a non-key, non-window, non-notification message
// that should fall through to default routing.
type customDefaultMsg struct{}

func TestUpdate_DefaultRoutingFeedsToastAndScreen(t *testing.T) {
	m := newTestShell(t)
	rec := &recordingScreen{}
	m.stack[0] = rec

	msg := customDefaultMsg{}
	_, cmd := m.Update(msg)

	if len(rec.received) != 1 || rec.received[0] != msg {
		t.Fatalf("recording screen received %v, want one customDefaultMsg", rec.received)
	}
	// Toast.Update returns nil cmd for foreign messages, so cmd may
	// be a nil-cmd batch. Asserting that the screen saw the message
	// is the load-bearing claim; cmd shape is implementation detail.
	_ = cmd
}

func TestUpdate_NotificationMsgFeedsLog(t *testing.T) {
	m := newTestShell(t)

	n := notifications.Notification{
		Severity:  errs.SeverityInfo,
		Text:      "profile created",
		CreatedAt: time.Now(),
	}

	tm, _ := m.Update(notifications.NotificationMsg{Notification: n})
	m = tm.(Model)

	entries := m.log.Entries()
	if len(entries) != 1 {
		t.Fatalf("log has %d entries, want 1", len(entries))
	}
}

func TestUpdate_NotificationMsgFeedsToast(t *testing.T) {
	m := newTestShell(t)

	n := notifications.Notification{
		Severity:  errs.SeverityInfo,
		Text:      "profile created",
		CreatedAt: time.Now(),
	}

	tm, _ := m.Update(notifications.NotificationMsg{Notification: n})
	m = tm.(Model)

	if m.toast.Empty() {
		t.Fatalf("toast empty after NotificationMsg, want non-empty")
	}
}

// View reserves a single toast line even when no notification is active
// so the status bar never jumps as toasts arrive and expire.
func TestView_ReservesToastSlotWhenEmpty(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)

	if !m.toast.Empty() {
		t.Fatalf("precondition: toast should be empty")
	}

	got := strings.Count(m.View().Content, "\n") + 1

	// Re-render with an active toast: same line count.
	n := notifications.Notification{
		Severity:  errs.SeverityInfo,
		Text:      "x",
		CreatedAt: time.Now(),
	}
	tm, _ = m.Update(notifications.NotificationMsg{Notification: n})
	m = tm.(Model)
	withToast := strings.Count(m.View().Content, "\n") + 1
	if got != withToast {
		t.Errorf("layout height changed: empty=%d, with-toast=%d", got, withToast)
	}
}

// sizingSpy records every WindowSizeMsg it receives.
type sizingSpy struct {
	received []tea.WindowSizeMsg
}

func (s *sizingSpy) Init() tea.Cmd { return nil }
func (s *sizingSpy) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.received = append(s.received, ws)
	}
	return s, nil
}
func (s *sizingSpy) Body(_ int) string         { return "" }
func (s *sizingSpy) Title() string             { return "spy" }
func (s *sizingSpy) StatusKeys() []key.Binding { return nil }
func (s *sizingSpy) InputFocused() bool        { return false }

// sizingSpy2 is a distinct concrete type from sizingSpy so push-dedup
// (which compares concrete types) allows stacking the two for the
// buried-screen propagation test.
type sizingSpy2 struct{ sizingSpy }

func TestUpdate_WindowSizeStoredAndPropagatedToTop(t *testing.T) {
	m := newTestShell(t)
	spy := &sizingSpy{}
	m.stack[0] = spy

	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = tm.(Model)

	if m.width != 120 || m.height != 40 {
		t.Fatalf("shell stored (w=%d,h=%d), want (120,40)", m.width, m.height)
	}
	if len(spy.received) != 1 {
		t.Fatalf("top screen received %d resize msgs, want 1", len(spy.received))
	}
	if spy.received[0].Width != 120 || spy.received[0].Height != 40 {
		t.Fatalf("top got %v, want {120,40}", spy.received[0])
	}
}

func TestUpdate_WindowSizePropagatesToBuriedScreen(t *testing.T) {
	m := newTestShell(t)
	root := &sizingSpy{}
	m.stack[0] = root

	pushed := &sizingSpy2{}
	tm, _ := m.Update(PushScreenMsg{Screen: pushed})
	m = tm.(Model)
	if len(m.stack) != 2 {
		t.Fatalf("push dedup unexpectedly applied; depth=%d", len(m.stack))
	}

	tm, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = tm.(Model)

	if len(root.received) != 1 {
		t.Errorf("buried root received %d resize msgs, want 1", len(root.received))
	}
	if len(pushed.received) != 1 {
		t.Errorf("top received %d resize msgs, want 1", len(pushed.received))
	}
}

func TestRenderStatusBar_IncludesGlobalAndDynamic(t *testing.T) {
	global := defaultGlobalKeyMap()
	edit := key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	del := key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete"))

	got := renderStatusBar(global, []key.Binding{edit, del})

	for _, want := range []string{
		"n notifications",
		"s settings",
		"q quit",
		"? help",
		"↑/k up",
		"↓/j down",
		"e edit",
		"d delete",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status bar missing %q\n%s", want, got)
		}
	}
}

func TestRenderStatusBar_OmitsScreenLevelVerbsWhenDynamicEmpty(t *testing.T) {
	global := defaultGlobalKeyMap()

	got := renderStatusBar(global, nil)

	for _, banned := range []string{"c create", "r register", "b back"} {
		if strings.Contains(got, banned) {
			t.Errorf("status bar leaked screen-level verb %q\n%s", banned, got)
		}
	}
}

func TestView_ContainsTitleAndStatusBarHints(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)

	v := m.View()
	content := v.Content

	if !strings.Contains(content, "Agentfiles") {
		t.Errorf("view missing title 'Agentfiles':\n%s", content)
	}
	for _, want := range []string{"n notifications", "s settings", "q quit", "? help"} {
		if !strings.Contains(content, want) {
			t.Errorf("view missing status hint %q", want)
		}
	}
	if !v.AltScreen {
		t.Errorf("view AltScreen = false, want true")
	}
}

// bodyWidthSpy records the width Body was last called with.
type bodyWidthSpy struct {
	width int
}

func (s *bodyWidthSpy) Init() tea.Cmd                      { return nil }
func (s *bodyWidthSpy) Update(_ tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (s *bodyWidthSpy) Title() string                      { return "size" }
func (s *bodyWidthSpy) StatusKeys() []key.Binding          { return nil }
func (s *bodyWidthSpy) InputFocused() bool                 { return false }
func (s *bodyWidthSpy) Body(w int) string {
	s.width = w
	return ""
}

func TestView_BodyReceivesWindowWidth(t *testing.T) {
	m := newTestShell(t)
	spy := &bodyWidthSpy{}
	m.stack[0] = spy

	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)

	_ = m.View()

	if spy.width != 80 {
		t.Errorf("Body width = %d, want 80", spy.width)
	}
}
