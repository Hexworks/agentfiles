package shell

import (
	"reflect"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// toaster is the slice of *notifications.Toast the shell consumes.
// Update returns toaster (not the concrete *Toast) so the field can
// stay typed as the interface across the loop's reassignment.
type toaster interface {
	Update(tea.Msg) (toaster, tea.Cmd)
	View() string
	Empty() bool
}

// Model is the root tea.Model. It owns the screen stack, the toast
// widget, and the persistent notifications log; the actions handle
// is held so future screens (tasks 0024+) can request domain work
// without reaching into app.Service directly.
type Model struct {
	actions *actions.Actions
	toast   toaster
	log     *notifications.Log

	keys  globalKeyMap
	stack []Screen

	width, height int
}

// toastAdapter wraps the concrete *notifications.Toast so its
// Update can satisfy the local toaster interface (which returns
// `toaster`, not `*notifications.Toast`).
type toastAdapter struct{ inner *notifications.Toast }

func (t *toastAdapter) Update(msg tea.Msg) (toaster, tea.Cmd) {
	inner, cmd := t.inner.Update(msg)
	t.inner = inner
	return t, cmd
}
func (t *toastAdapter) View() string { return t.inner.View() }
func (t *toastAdapter) Empty() bool  { return t.inner.Empty() }

// New constructs the shell, mounts a fresh Toast (default 5 s
// duration), wires the supplied Log, and seeds the stack with the
// Welcome screen. The actions handle is non-nil; the log is non-nil.
// New panics on nil inputs because both are programmer errors caught
// at wiring time, not runtime conditions to recover from.
func New(a *actions.Actions, log *notifications.Log) Model {
	if a == nil {
		panic("shell.New: nil actions")
	}
	if log == nil {
		panic("shell.New: nil log")
	}
	keys := defaultGlobalKeyMap()
	return Model{
		actions: a,
		toast:   &toastAdapter{inner: notifications.NewToast(0)},
		log:     log,
		keys:    keys,
		stack:   []Screen{newWelcomeScreen(keys, a)},
	}
}

// Init runs the root-of-stack screen's Init.
func (m Model) Init() tea.Cmd {
	return m.stack[len(m.stack)-1].Init()
}

// Update routes messages by message type: window resize propagates to
// every screen on the stack; key presses go through the global key
// map first and reach the active screen only when nothing global
// matched; NotificationMsg fans out to log + toast; PushScreenMsg /
// PopScreenMsg mutate the stack; everything else broadcasts to the
// toast (which ignores foreign messages) and the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		cmds := make([]tea.Cmd, len(m.stack))
		for i, s := range m.stack {
			updated, cmd := s.Update(msg)
			m.stack[i] = updated
			cmds[i] = cmd
		}
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		if cmd, handled := m.handleGlobalKey(msg); handled {
			return m, cmd
		}

	case notifications.NotificationMsg:
		return m.notify(msg.Notification)

	case PushScreenMsg:
		return m.pushScreen(msg.Screen)

	case PopScreenMsg:
		return m.popScreen()
	}

	// Toast.Update has to run on every unhandled message because its
	// internal expireMsg is unexported in notifications — the shell
	// cannot name it in a type switch. Toast ignores foreign
	// messages, so this broadcast is safe.
	t, tCmd := m.toast.Update(msg)
	m.toast = t

	top := len(m.stack) - 1
	s, sCmd := m.stack[top].Update(msg)
	m.stack[top] = s

	return m, tea.Batch(tCmd, sCmd)
}

// notify fans a Notification out to the persistent log and the toast
// queue. Centralizing the rule means a future screen-level filter or
// dedup policy lands here, not inline in Update.
//
// The toast queue itself is intentionally uncapped. Notifications are
// produced by user-initiated actions (one toast per action result),
// so growth is bounded by human key-press cadence rather than program
// throughput. Adding a cap would force a "lost notification" UX for a
// case the project's interaction model never reaches; if a future
// background action ever emits notifications at machine speed,
// introduce the cap then.
func (m Model) notify(n notifications.Notification) (Model, tea.Cmd) {
	m.log.Add(n)
	t, cmd := m.toast.Update(notifications.PushMsg{Notification: n})
	m.toast = t
	return m, cmd
}

// pushScreen appends s to the stack and runs its Init.
func (m Model) pushScreen(s Screen) (Model, tea.Cmd) {
	top := m.stack[len(m.stack)-1]
	if sameScreenType(top, s) {
		return m, nil
	}
	m.stack = append(m.stack, s)
	return m, s.Init()
}

// popScreen removes the top screen. On a single-screen stack it is a
// no-op (root cannot be popped). The popped slot is zeroed before
// shrinking so the backing array no longer holds the screen.
func (m Model) popScreen() (Model, tea.Cmd) {
	if len(m.stack) <= 1 {
		return m, nil
	}
	i := len(m.stack) - 1
	m.stack[i] = nil
	m.stack = m.stack[:i]
	return m, nil
}

func sameScreenType(a, b Screen) bool {
	return a != nil && b != nil && reflect.TypeOf(a) == reflect.TypeOf(b)
}

// View composes title + body + toast + status bar into an alt-screen
// tea.View. Body receives the dimensions the shell has not reserved
// for the surrounding chrome; on a zero or negative remainder the
// body height clamps to 0.
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
