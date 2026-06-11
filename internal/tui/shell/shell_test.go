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
	if _, ok := m.stack[0].(welcomeStub); !ok {
		t.Fatalf("initial top = %T, want welcomeStub", m.stack[0])
	}

	tm, _ := m.Update(PushScreenMsg{Screen: newSettingsStub()})
	m = tm.(Model)
	if len(m.stack) != 2 {
		t.Fatalf("after push depth = %d, want 2", len(m.stack))
	}
	if _, ok := m.stack[1].(settingsStub); !ok {
		t.Fatalf("top after push = %T, want settingsStub", m.stack[1])
	}
}

func TestUpdate_PopScreenMsgShrinksStack(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(PushScreenMsg{Screen: newSettingsStub()})
	m = tm.(Model)

	tm, _ = m.Update(PopScreenMsg{})
	m = tm.(Model)
	if len(m.stack) != 1 {
		t.Fatalf("after pop depth = %d, want 1", len(m.stack))
	}
	if _, ok := m.stack[0].(welcomeStub); !ok {
		t.Fatalf("top after pop = %T, want welcomeStub", m.stack[0])
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
// keys).
type recordingScreen struct {
	received []tea.Msg
}

func (s *recordingScreen) Init() tea.Cmd { return nil }
func (s *recordingScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}
func (s *recordingScreen) Body(_ int, _ int) string  { return "" }
func (s *recordingScreen) Title() string             { return "rec" }
func (s *recordingScreen) StatusKeys() []key.Binding { return nil }

func TestUpdate_GlobalKeysInterceptedBeforeScreen(t *testing.T) {
	cases := []struct {
		name    string
		msg     tea.KeyPressMsg
		wantCmd string // "push:<title>" or "quit"
	}{
		{"n notifications", tea.KeyPressMsg{Code: 'n', Text: "n"}, "push:Notifications"},
		{"s settings", tea.KeyPressMsg{Code: 's', Text: "s"}, "push:Settings"},
		{"? help", tea.KeyPressMsg{Code: '?', Text: "?"}, "push:Info"},
		{"q quit", tea.KeyPressMsg{Code: 'q', Text: "q"}, "quit"},
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

func TestUpdate_CtrlCQuits(t *testing.T) {
	m := newTestShell(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl, Text: ""})
	if cmd == nil {
		t.Fatalf("expected quit cmd, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c produced %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdate_NotificationMsgFeedsLogAndToast(t *testing.T) {
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
	if entries[0].Text != "profile created" {
		t.Fatalf("log[0].Text = %q, want %q", entries[0].Text, "profile created")
	}
	if m.toast.Empty() {
		t.Fatalf("toast empty after NotificationMsg, want non-empty")
	}
}

// sizingSpy records the last WindowSizeMsg it receives.
type sizingSpy struct {
	last tea.WindowSizeMsg
	got  bool
}

func (s *sizingSpy) Init() tea.Cmd { return nil }
func (s *sizingSpy) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.last = ws
		s.got = true
	}
	return s, nil
}
func (s *sizingSpy) Body(_ int, _ int) string  { return "" }
func (s *sizingSpy) Title() string             { return "spy" }
func (s *sizingSpy) StatusKeys() []key.Binding { return nil }

func TestUpdate_WindowSizeStoredAndPropagated(t *testing.T) {
	m := newTestShell(t)
	spy := &sizingSpy{}
	m.stack[0] = spy

	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = tm.(Model)

	if m.width != 120 || m.height != 40 {
		t.Fatalf("shell stored (w=%d,h=%d), want (120,40)", m.width, m.height)
	}
	if !spy.got {
		t.Fatalf("active screen did not receive WindowSizeMsg")
	}
	if spy.last.Width != 120 || spy.last.Height != 40 {
		t.Fatalf("screen got %v, want {120,40}", spy.last)
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

	// Screen-level button verbs that the rule says must NOT be
	// duplicated in the bar — the screen would surface them by
	// putting them into StatusKeys, which is exactly what the rule
	// forbids. With an empty dynamic set, these must not appear.
	emptyBar := renderStatusBar(global, nil)
	for _, banned := range []string{"c create", "r register", "b back"} {
		if strings.Contains(emptyBar, banned) {
			t.Errorf("status bar leaked screen-level verb %q\n%s", banned, emptyBar)
		}
	}
}

func TestView_ContainsTitleAndStatusBarHints(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)

	v := m.View()
	content := v.Content

	if !strings.Contains(content, "Welcome") {
		t.Errorf("view missing title 'Welcome':\n%s", content)
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
