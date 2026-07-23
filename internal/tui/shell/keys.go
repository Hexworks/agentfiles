package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/help"
)

// globalKeyMap holds the bindings the shell intercepts before the
// active screen sees the key press, plus the navigation hints that
// are display-only (Up/Down are owned by individual screens but the
// status bar still advertises them so the user knows how to move).
type globalKeyMap struct {
	Notifications key.Binding // n  → emit ShowNotificationsMsg (shell-owned overlay)
	Settings      key.Binding // s  → push settingsScreen
	Help          key.Binding // ?  → emit ShowHelpMsg (shell-owned overlay)
	Quit          key.Binding // q  / ctrl+c → tea.Quit
	Up            key.Binding // ↑/k — display-only
	Down          key.Binding // ↓/j — display-only
}

func defaultGlobalKeyMap() globalKeyMap {
	return globalKeyMap{
		Notifications: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "notifications"),
		),
		Settings: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "settings"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			// ctrl+c is bundled with q so the unconditional-abort
			// path goes through the same key.Matches discipline as
			// every other global key — see ADR 0011.
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
	}
}

// helpTopic resolves the manual topic for the active screen at the
// moment `?` was pressed. Screens that satisfy [help.Topical] supply
// their own; everything else falls back to [help.OverviewTopic].
func (m Model) helpTopic() help.Topic {
	focus := m.stack[len(m.stack)-1]
	return help.TopicFor(focus)
}

// handleGlobalKey matches kp against the global set. On hit it
// returns the corresponding command and reports handled=true so the
// shell's Update can short-circuit and the active screen never sees
// the key.
func (m Model) handleGlobalKey(kp tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(kp, m.keys.Notifications):
		return showNotificationsCmd(), true
	case key.Matches(kp, m.keys.Settings):
		return pushCmd(newSettingsScreen(m.actions)), true
	case key.Matches(kp, m.keys.Help):
		return showHelpCmd(m.helpTopic()), true
	case key.Matches(kp, m.keys.Quit):
		return tea.Quit, true
	}
	return nil, false
}
