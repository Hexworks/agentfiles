package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/notificationsmodal"
)

// notificationsScreen wraps the [notificationsmodal.Modal] returned from the
// component package. It exists so the shell's Screen stack can host the
// modal without the shell needing a native overlay layer: every render
// the modal's bordered view fills the screen body, and a ResolvedMsg
// pops the screen.
type notificationsScreen struct {
	modal *modal.Modal
}

// newNotificationsScreen mounts the Notifications modal using the
// shell's current viewport so the table sizes itself to the open
// window. The log read snapshot happens inside [notificationsmodal.New].
func (m Model) newNotificationsScreen() *notificationsScreen {
	w, h := modalSize(m.width, m.height)
	return &notificationsScreen{
		modal: notificationsmodal.New("notifications", m.log, w, h),
	}
}

func (s *notificationsScreen) Init() tea.Cmd { return s.modal.Init() }

func (s *notificationsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if _, ok := msg.(modal.ResolvedMsg); ok {
		return s, popCmd()
	}
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		w, h := modalSize(ws.Width, ws.Height)
		s.modal.SetSize(w, h)
	}
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

func (s *notificationsScreen) Body(_ int) string { return s.modal.View() }
func (s *notificationsScreen) Title() string     { return "Notifications" }
func (s *notificationsScreen) Topic() help.Topic {
	return help.Topic{Label: "Notifications", File: "notifications.md"}
}
func (s *notificationsScreen) StatusKeys() []key.Binding { return nil }
func (s *notificationsScreen) InputFocused() bool        { return s.modal.Active() }
