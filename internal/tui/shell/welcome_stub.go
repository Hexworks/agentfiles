package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// welcomeStub is the placeholder root screen pushed by [New]. It
// renders a single greeting line; the real Welcome screen arrives in
// task 0024.
type welcomeStub struct{}

func newWelcomeStub() *welcomeStub { return &welcomeStub{} }

func (s *welcomeStub) Init() tea.Cmd                      { return nil }
func (s *welcomeStub) Update(_ tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (s *welcomeStub) Body(_ int, _ int) string           { return "agentfiles — press q to quit" }
func (s *welcomeStub) Title() string                      { return "Welcome" }
func (s *welcomeStub) StatusKeys() []key.Binding          { return nil }
