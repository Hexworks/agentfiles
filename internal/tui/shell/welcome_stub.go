package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// welcomeStub is the placeholder root screen pushed by [New]. It
// renders a single greeting line; the real Welcome screen arrives in
// task 0024.
type welcomeStub struct{}

func newWelcomeStub() welcomeStub { return welcomeStub{} }

func (welcomeStub) Init() tea.Cmd                        { return nil }
func (s welcomeStub) Update(_ tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (welcomeStub) Body(_ int, _ int) string             { return "agentfiles — press q to quit" }
func (welcomeStub) Title() string                        { return "Welcome" }
func (welcomeStub) StatusKeys() []key.Binding            { return nil }

// notificationsStub stands in for the Notifications modal (task 0023).
// `esc` pops it; `q` is consumed by the shell as Quit.
type notificationsStub struct{}

func newNotificationsStub() notificationsStub { return notificationsStub{} }

func (s notificationsStub) Init() tea.Cmd { return nil }
func (s notificationsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "esc" {
		return s, popCmd()
	}
	return s, nil
}
func (notificationsStub) Body(_ int, _ int) string { return "(notifications modal — task 0023)" }
func (notificationsStub) Title() string            { return "Notifications" }
func (notificationsStub) StatusKeys() []key.Binding {
	return nil
}

// settingsStub stands in for the Settings screen (task 0024).
type settingsStub struct{}

func newSettingsStub() settingsStub { return settingsStub{} }

func (s settingsStub) Init() tea.Cmd { return nil }
func (s settingsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "esc" {
		return s, popCmd()
	}
	return s, nil
}
func (settingsStub) Body(_ int, _ int) string  { return "(settings screen — task 0024)" }
func (settingsStub) Title() string             { return "Settings" }
func (settingsStub) StatusKeys() []key.Binding { return nil }

// infoStub stands in for the Info modal (task 0023).
type infoStub struct{}

func newInfoStub() infoStub { return infoStub{} }

func (s infoStub) Init() tea.Cmd { return nil }
func (s infoStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "esc" {
		return s, popCmd()
	}
	return s, nil
}
func (infoStub) Body(_ int, _ int) string  { return "(info modal — task 0023)" }
func (infoStub) Title() string             { return "Info" }
func (infoStub) StatusKeys() []key.Binding { return nil }
