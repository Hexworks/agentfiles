package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// profilesStub is the placeholder Profiles screen pushed by the
// Welcome menu until task 0025 lands the real screen. `esc` pops it;
// `q` is consumed by the shell as Quit.
type profilesStub struct{}

func newProfilesStub() *profilesStub { return &profilesStub{} }

func (s *profilesStub) Init() tea.Cmd { return nil }

func (s *profilesStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.Code == tea.KeyEsc {
		return s, popCmd()
	}
	return s, nil
}

func (s *profilesStub) Body(_ int, _ int) string  { return "(profiles screen — task 0025)" }
func (s *profilesStub) Title() string             { return "Profiles" }
func (s *profilesStub) StatusKeys() []key.Binding { return nil }
