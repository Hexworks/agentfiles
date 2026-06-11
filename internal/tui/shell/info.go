package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// infoScreen wraps the markdown help modal so the shell's Screen stack
// can mount it. The topic is resolved at push time from the focused
// screen (see topicFor); reopening the modal re-resolves the topic so
// any later registry changes take effect.
type infoScreen struct {
	modal *modal.Modal
}

// newInfoScreen mounts the help modal with the topic registered for the
// screen the user is looking at. With today's MVP that always returns
// the overview, but the dispatch site is kept consistent with future
// per-screen overrides.
func (m Model) newInfoScreen() *infoScreen {
	w, h := modalSize(m.width, m.height)
	focus := m.stack[len(m.stack)-1]
	req := help.Request{Topic: "Info", Path: topicFor(focus)}
	return &infoScreen{
		modal: help.New("info", req, w, h),
	}
}

func (s *infoScreen) Init() tea.Cmd { return s.modal.Init() }

func (s *infoScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if _, ok := msg.(modal.ResolvedMsg); ok {
		return s, popCmd()
	}
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

func (s *infoScreen) Body(_ int, _ int) string  { return s.modal.View() }
func (s *infoScreen) Title() string             { return "Info" }
func (s *infoScreen) StatusKeys() []key.Binding { return nil }

// modalSize returns the (width, height) the shell hands to a modal
// constructor. A small floor protects modals from a pre-WindowSize
// open: until tea sends the first resize, shell.Model.width/height are
// zero. Reserving four rows for the title bar (3) + status bar (1)
// matches the shell's layout in shell.go::View.
func modalSize(width, height int) (int, int) {
	w := width
	if w < 40 {
		w = 40
	}
	h := height - 4
	if h < 10 {
		h = 10
	}
	return w, h
}
