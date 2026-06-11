package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// notificationsStub stands in for the Notifications modal (task 0023).
// `esc` pops it; `q` is consumed by the shell as Quit.
type notificationsStub struct{}

func newNotificationsStub() *notificationsStub { return &notificationsStub{} }

func (s *notificationsStub) Init() tea.Cmd { return nil }

func (s *notificationsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.Code == tea.KeyEsc {
		return s, popCmd()
	}
	return s, nil
}

func (s *notificationsStub) Body(_ int, _ int) string  { return "(notifications modal — task 0023)" }
func (s *notificationsStub) Title() string             { return "Notifications" }
func (s *notificationsStub) StatusKeys() []key.Binding { return nil }
