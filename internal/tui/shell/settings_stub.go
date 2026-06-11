package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// settingsStub stands in for the Settings screen (task 0024). `esc`
// pops it; `q` is consumed by the shell as Quit.
type settingsStub struct{}

func newSettingsStub() *settingsStub { return &settingsStub{} }

func (s *settingsStub) Init() tea.Cmd { return nil }

func (s *settingsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.Code == tea.KeyEsc {
		return s, popCmd()
	}
	return s, nil
}

func (s *settingsStub) Body(_ int, _ int) string  { return "(settings screen — task 0024)" }
func (s *settingsStub) Title() string             { return "Settings" }
func (s *settingsStub) StatusKeys() []key.Binding { return nil }
