package shell

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// Model is the root tea.Model. It owns the screen stack, the toast
// widget, and the persistent notifications log; the actions handle
// is held so future screens (tasks 0024+) can request domain work
// without reaching into app.Service directly.
type Model struct {
	actions *actions.Actions
	toast   *notifications.Toast
	log     *notifications.Log

	keys  globalKeyMap
	stack []Screen

	width, height int
}

// New constructs the shell, mounts a fresh Toast (default 5 s
// duration), wires the supplied Log, and seeds the stack with the
// placeholder Welcome screen. The actions handle is non-nil; the log
// is non-nil. New panics on nil inputs because both are programmer
// errors caught at wiring time, not runtime conditions to recover
// from.
func New(a *actions.Actions, log *notifications.Log) Model {
	if a == nil {
		panic("shell.New: nil actions")
	}
	if log == nil {
		panic("shell.New: nil log")
	}
	return Model{
		actions: a,
		toast:   notifications.NewToast(0),
		log:     log,
		keys:    defaultGlobalKeyMap(),
		stack:   []Screen{newWelcomeStub()},
	}
}

func (m Model) Init() tea.Cmd {
	return m.stack[len(m.stack)-1].Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		top := len(m.stack) - 1
		s, cmd := m.stack[top].Update(msg)
		m.stack[top] = s
		return m, cmd

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if cmd, handled := m.handleGlobalKey(msg); handled {
			return m, cmd
		}

	case notifications.NotificationMsg:
		m.log.Add(msg.Notification)
		t, cmd := m.toast.Update(notifications.PushMsg{Notification: msg.Notification})
		m.toast = t
		return m, cmd

	case PushScreenMsg:
		m.stack = append(m.stack, msg.Screen)
		return m, msg.Screen.Init()

	case PopScreenMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil
	}

	// Default routing: feed the toast (it ignores anything that
	// isn't PushMsg/expireMsg) and forward to the active screen.
	t, tCmd := m.toast.Update(msg)
	m.toast = t

	top := len(m.stack) - 1
	s, sCmd := m.stack[top].Update(msg)
	m.stack[top] = s

	return m, tea.Batch(tCmd, sCmd)
}

func (m Model) View() tea.View {
	top := m.stack[len(m.stack)-1]

	title := renderTitle(top.Title())
	toast := m.toast.View()
	status := renderStatusBar(m.keys, top.StatusKeys())

	headerH := lipgloss.Height(title)
	toastH := 0
	if toast != "" {
		toastH = lipgloss.Height(toast)
	}
	bodyH := m.height - headerH - toastH - lipgloss.Height(status)
	if bodyH < 0 {
		bodyH = 0
	}

	body := top.Body(m.width, bodyH)

	sections := []string{title, body}
	if toast != "" {
		sections = append(sections, toast)
	}
	sections = append(sections, status)

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
	v.AltScreen = true
	return v
}

var titleStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(0, 2).
	Inherit(styles.HeaderStyle)

func renderTitle(s string) string {
	return titleStyle.Render(s)
}
