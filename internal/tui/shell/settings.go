package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// settingsScreen is the MVP Settings screen: a single
// "Coming soon." placeholder line plus the [Back] mnemonic button. The
// real screen body lands in a later task; the screen type ships now so
// the Welcome menu and the global `s` shortcut have a stable target.
type settingsScreen struct {
	back *mnemonic.Button
}

func newSettingsScreen() *settingsScreen {
	return &settingsScreen{
		back: mnemonic.New(
			"Back",
			'b',
			func() tea.Cmd { return popCmd() },
			mnemonic.WithExtraBindingKeys("esc"),
		),
	}
}

func (s *settingsScreen) Init() tea.Cmd { return nil }

func (s *settingsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	if s.back.Matches(kp) {
		return s, s.back.Trigger()
	}
	return s, nil
}

func (s *settingsScreen) Title() string { return "Settings" }

// StatusKeys exposes [Back] to the status bar. The "screen-level
// buttons not duplicated" rule treats [Back] as a deliberate
// exception: there is no other visual cue for it on this otherwise
// empty body, so the bar is the only place the user discovers it.
func (s *settingsScreen) StatusKeys() []key.Binding {
	return []key.Binding{s.back.Binding()}
}

func (s *settingsScreen) Body(width, _ int) string {
	msg := " Coming soon."
	back := lipgloss.PlaceHorizontal(width, lipgloss.Right, s.back.View())
	return lipgloss.JoinVertical(lipgloss.Left, msg, "", back)
}
