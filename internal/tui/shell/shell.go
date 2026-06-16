package shell

import (
	"reflect"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
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
//
// helpModal is the manual-page overlay opened with `?`. It is owned at
// the shell level (rather than living on the stack as a screen) so that
// the global key handler can keep `q` wired to quit while the dialog is
// visible. nil = closed.
type Model struct {
	actions *actions.Actions
	toast   toaster
	log     *notifications.Log

	keys      globalKeyMap
	stack     []Screen
	helpModal *modal.Modal

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
		if m.helpModal.Active() {
			w, h := modalSize(msg.Width, msg.Height)
			m.helpModal.SetSize(w, h)
		}
		cmds := make([]tea.Cmd, len(m.stack))
		for i, s := range m.stack {
			updated, cmd := s.Update(msg)
			m.stack[i] = updated
			cmds[i] = cmd
		}
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		if m.helpModal.Active() {
			return m.routeHelpKey(msg)
		}
		top := m.stack[len(m.stack)-1]
		if !screenWantsRawKey(top, msg) {
			if cmd, handled := m.handleGlobalKey(msg); handled {
				return m, cmd
			}
		}

	case ShowHelpMsg:
		return m.openHelp(msg.Topic)

	case modal.ResolvedMsg:
		if msg.ID == helpModalID {
			m.helpModal = nil
			return m, nil
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

// pushScreen appends s to the stack and runs its Init. When the shell
// already has a window size, a synthetic WindowSizeMsg is batched with
// the Init command so the new screen learns its dimensions immediately
// instead of having to wait for the next user-driven resize event.
func (m Model) pushScreen(s Screen) (Model, tea.Cmd) {
	top := m.stack[len(m.stack)-1]
	if sameScreenType(top, s) {
		return m, nil
	}
	m.stack = append(m.stack, s)
	cmd := s.Init()
	if m.width <= 0 || m.height <= 0 {
		return m, cmd
	}
	width, height := m.width, m.height
	sizeCmd := func() tea.Msg { return tea.WindowSizeMsg{Width: width, Height: height} }
	if cmd == nil {
		return m, sizeCmd
	}
	return m, tea.Batch(cmd, sizeCmd)
}

// helpModalID is the modal.Modal ID used for the shell-owned manual
// overlay. Stored as a constant so the open path and the ResolvedMsg
// filter in Update cannot drift.
const helpModalID = "help"

// openHelp mounts the manual-page overlay for topic. The dialog is
// owned by the shell rather than pushed onto the screen stack so the
// global key handler can keep `q` mapped to quit while it is visible.
// A second `?` press while the modal is open is a no-op.
func (m Model) openHelp(topic help.Topic) (Model, tea.Cmd) {
	if m.helpModal.Active() {
		return m, nil
	}
	w, h := modalSize(m.width, m.height)
	req := help.Request{Topic: topic.Label, Path: topic.File}
	m.helpModal = help.New(helpModalID, req, w, h)
	return m, m.helpModal.Init()
}

// routeHelpKey dispatches a key press while the help modal is open.
// `q` and `ctrl+c` reach the shell-level quit binding so the user can
// always exit the application; every other key is forwarded to the
// modal (Esc cancels; ↑/↓/j/k scroll the viewport).
func (m Model) routeHelpKey(kp tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.Matches(kp, m.keys.Quit) {
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.helpModal, cmd = m.helpModal.Update(kp)
	return m, cmd
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
// tea.View. Body renders at its natural height; the shell does not pad
// it to fill the remaining viewport.
func (m Model) View() tea.View {
	top := m.stack[len(m.stack)-1]

	title := renderTitle(top.Title())
	toast := m.toast.View()
	status := renderStatusBar(m.keys, top.StatusKeys())

	body := top.Body(m.width)
	if m.helpModal.Active() {
		canvasH := m.bodyCanvasHeight(body)
		body = padBodyHeight(body, m.width, canvasH)
		body = m.helpModal.Render(body, m.width, canvasH)
	}

	// Toast slot is always reserved so the status bar does not jump
	// vertically when a notification arrives or expires. Empty toast
	// becomes a single blank line, matching the single-line height of
	// every rendered notification.
	if toast == "" {
		toast = " "
	}
	sections := []string{title, body, toast, status}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
	v.AltScreen = true
	return v
}

func renderTitle(s string) string {
	return styles.ShellTitleStyle.Render(s)
}

// bodyCanvasHeight returns the height of the canvas the help overlay
// should center on: the visible body area after chrome and the
// always-reserved toast slot, falling back to the natural body height
// when the viewport size has not been seen yet (m.height == 0).
func (m Model) bodyCanvasHeight(body string) int {
	if m.height <= 0 {
		return lipgloss.Height(body)
	}
	avail := m.height - chromeHeight
	if avail < 1 {
		avail = 1
	}
	if h := lipgloss.Height(body); h > avail {
		return h
	}
	return avail
}

// padBodyHeight grows body to canvasH rows with blank lines so the
// help overlay can center inside the visible body area instead of
// collapsing to the screen's natural (short) height. No-op when the
// body already meets or exceeds canvasH.
func padBodyHeight(body string, width, canvasH int) string {
	gap := canvasH - lipgloss.Height(body)
	if gap <= 0 {
		return body
	}
	filler := lipgloss.NewStyle().Width(width).Render("")
	pad := make([]string, gap+1)
	pad[0] = body
	for i := 1; i <= gap; i++ {
		pad[i] = filler
	}
	return lipgloss.JoinVertical(lipgloss.Left, pad...)
}
