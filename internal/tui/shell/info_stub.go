package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// infoStub stands in for the Info modal (task 0023). `esc` pops it;
// `q` is consumed by the shell as Quit.
type infoStub struct{}

func newInfoStub() *infoStub { return &infoStub{} }

func (s *infoStub) Init() tea.Cmd { return nil }

func (s *infoStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.Code == tea.KeyEsc {
		return s, popCmd()
	}
	return s, nil
}

func (s *infoStub) Body(_ int, _ int) string  { return "(info modal — task 0023)" }
func (s *infoStub) Title() string             { return "Info" }
func (s *infoStub) StatusKeys() []key.Binding { return nil }
