package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// welcomeItem is one row in the Welcome menu: a visible label and the
// command fired when the row is chosen. Storing the action as a
// constructor (rather than a cached tea.Cmd) means each invocation
// pushes a fresh screen instance, so a popped+re-pushed entity screen
// starts clean.
type welcomeItem struct {
	label  string
	action func() tea.Cmd
}

// welcomeScreen is the root menu the shell seeds. It owns a small
// vertical-list cursor over a fixed route set (Profiles / Settings /
// Quit). Arrow keys and k/j move the cursor with wrap-around; enter
// and v fire the selected row's action.
type welcomeScreen struct {
	cursor int
	items  []welcomeItem

	up     key.Binding
	down   key.Binding
	choose key.Binding
}

func newWelcomeScreen() *welcomeScreen {
	return &welcomeScreen{
		items: []welcomeItem{
			{label: "Profiles", action: func() tea.Cmd { return pushCmd(newProfilesStub()) }},
			{label: "Settings", action: func() tea.Cmd { return pushCmd(newSettingsScreen()) }},
			{label: "Quit", action: func() tea.Cmd { return tea.Quit }},
		},
		up:     key.NewBinding(key.WithKeys("up", "k")),
		down:   key.NewBinding(key.WithKeys("down", "j")),
		choose: key.NewBinding(key.WithKeys("enter", "v")),
	}
}

func (s *welcomeScreen) Init() tea.Cmd { return nil }

func (s *welcomeScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	n := len(s.items)
	switch {
	case key.Matches(kp, s.up):
		s.cursor = (s.cursor - 1 + n) % n
	case key.Matches(kp, s.down):
		s.cursor = (s.cursor + 1) % n
	case key.Matches(kp, s.choose):
		return s, s.items[s.cursor].action()
	}
	return s, nil
}

func (s *welcomeScreen) Title() string { return "Agentfiles" }

func (s *welcomeScreen) StatusKeys() []key.Binding { return nil }

func (s *welcomeScreen) Body(_ int, _ int) string {
	bar := styles.MutedStyle.Render("┃")
	rows := make([]string, 0, len(s.items)+1)
	rows = append(rows, bar+" "+styles.HeaderStyle.Render("Choose a task"))
	for i, it := range s.items {
		marker := "  "
		if i == s.cursor {
			marker = "> "
		}
		rows = append(rows, bar+" "+marker+it.label)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
