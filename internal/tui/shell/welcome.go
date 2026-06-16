package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
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
//
// Up and Down are reused from the shell's globalKeyMap so the status
// bar hint (display-only on the global map) cannot drift from the
// keys the screen actually matches.
type welcomeScreen struct {
	cursor int
	items  []welcomeItem

	up     key.Binding
	down   key.Binding
	choose key.Binding
}

func newWelcomeScreen(globals globalKeyMap, a *actions.Actions) *welcomeScreen {
	return &welcomeScreen{
		items: []welcomeItem{
			{label: "Profiles", action: func() tea.Cmd { return pushCmd(newProfilesScreen(a)) }},
			{label: "Settings", action: func() tea.Cmd { return pushCmd(newSettingsScreen()) }},
			{label: "Quit", action: func() tea.Cmd { return tea.Quit }},
		},
		up:     globals.Up,
		down:   globals.Down,
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

func (s *welcomeScreen) Description() string {
	return "Main menu — pick a task to get started"
}

func (s *welcomeScreen) StatusKeys() []key.Binding { return nil }

func (s *welcomeScreen) InputFocused() bool { return false }

func (s *welcomeScreen) Body(width int) string {
	bar := styles.MutedStyle.Render("┃")
	rows := make([]string, 0, len(s.items)+1)
	rows = append(rows, bar+" "+styles.HeaderStyle.Render(clampLabel("Choose a task", width-rowPrefixWidth)))
	for i, it := range s.items {
		marker := "  "
		if i == s.cursor {
			marker = "> "
		}
		rows = append(rows, bar+" "+styles.TextStyle.Render(marker+clampLabel(it.label, width-rowPrefixWidth)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// rowPrefixWidth is the visible width consumed by the bar, space, and
// selection marker that precede every label in the Welcome menu.
const rowPrefixWidth = 4

// clampLabel truncates label so its rendered width does not exceed max.
// A non-positive max leaves the label untouched (no WindowSize yet, or
// the prefix already consumes the entire row). Truncation appends an
// ellipsis when at least one rune of label still fits.
func clampLabel(label string, max int) string {
	if max <= 0 || lipgloss.Width(label) <= max {
		return label
	}
	runes := []rune(label)
	if max == 1 {
		return string(runes[:1])
	}
	return string(runes[:max-1]) + "…"
}
