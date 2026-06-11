package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// globalKeyMap holds the bindings the shell intercepts before the
// active screen sees the key press, plus the navigation hints that
// are display-only (Up/Down are owned by individual screens but the
// status bar still advertises them so the user knows how to move).
type globalKeyMap struct {
	Notifications key.Binding // n  → push notificationsStub
	Settings      key.Binding // s  → push settingsStub
	Help          key.Binding // ?  → push infoStub
	Quit          key.Binding // q  → tea.Quit
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
			key.WithKeys("q"),
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

// handleGlobalKey matches kp against the global set. On hit it
// returns the corresponding command and reports handled=true so the
// shell's Update can short-circuit and the active screen never sees
// the key.
func (m Model) handleGlobalKey(kp tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(kp, m.keys.Notifications):
		return pushCmd(newNotificationsStub()), true
	case key.Matches(kp, m.keys.Settings):
		return pushCmd(newSettingsStub()), true
	case key.Matches(kp, m.keys.Help):
		return pushCmd(newInfoStub()), true
	case key.Matches(kp, m.keys.Quit):
		return tea.Quit, true
	}
	return nil, false
}

func pushCmd(s Screen) tea.Cmd {
	return func() tea.Msg { return PushScreenMsg{Screen: s} }
}

func popCmd() tea.Cmd {
	return func() tea.Msg { return PopScreenMsg{} }
}
