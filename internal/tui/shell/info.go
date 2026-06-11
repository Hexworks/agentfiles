package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// infoScreen wraps the markdown help modal so the shell's Screen stack
// can mount it. The topic is resolved at push time from the focused
// screen via [help.TopicFor]; reopening the modal re-resolves the
// topic so any later registry changes take effect.
type infoScreen struct {
	modal *modal.Modal
}

// newInfoScreen mounts the help modal with the topic registered for the
// screen the user is looking at. Screens that satisfy [help.Topical]
// supply their own topic; everything else falls back to
// [help.OverviewTopic].
func (m Model) newInfoScreen() *infoScreen {
	w, h := modalSize(m.width, m.height)
	focus := m.stack[len(m.stack)-1]
	topic := help.TopicFor(focus)
	req := help.Request{Topic: topic.Label, Path: topic.File}
	return &infoScreen{
		modal: help.New("info", req, w, h),
	}
}

func (s *infoScreen) Init() tea.Cmd { return s.modal.Init() }

func (s *infoScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
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

func (s *infoScreen) Body(_ int, _ int) string  { return s.modal.View() }
func (s *infoScreen) Title() string             { return "Help" }
func (s *infoScreen) StatusKeys() []key.Binding { return nil }
