//go:build ignore

// Example program demonstrating the mnemonic.Button component.
//
// A mnemonic button is a `[Label]` widget bound to a single-character keyboard
// shortcut (the "mnemonic"). The mnemonic character is highlighted inside the
// label so the user can spot the activating key at a glance — e.g. `[Save]`
// renders the `S` underlined and bolded.
//
// Buttons are passive: they do not subscribe to Bubble Tea messages or own any
// internal state machine. The hosting model is responsible for:
//
//  1. Constructing each Button with a label, mnemonic rune, and Action.
//  2. Routing every tea.KeyPressMsg through Button.Matches, calling
//     Button.Trigger() on the first match.
//  3. Calling Button.View() wherever the button should appear in the layout.
//
// This split keeps mnemonic uniqueness (one shortcut per screen) and key
// routing the host's concern, not the component's.
//
// Run with: make example mnemonic
package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

type statusMsg string

func setStatus(s string) tea.Cmd {
	return func() tea.Msg { return statusMsg(s) }
}

type model struct {
	buttons []*mnemonic.Button
	status  string
}

func newModel() model {
	// Styles control how the three parts of a button render. The Mnemonic
	// style is applied to the first case-insensitive occurrence of the
	// mnemonic rune inside the label; everything else falls under Label,
	// and the surrounding `[` / `]` use Bracket.
	label := lipgloss.Color("13")
	hint := lipgloss.Color("99")
	styles := mnemonic.Styles{
		Bracket:  lipgloss.NewStyle().Bold(true).Foreground(label),
		Label:    lipgloss.NewStyle().Bold(true).Foreground(label),
		Mnemonic: lipgloss.NewStyle().Bold(true).Underline(true).Foreground(hint),
	}
	// mnemonic.New panics if the rune is absent from the label or if the
	// action is nil — these are programmer errors, caught at startup.
	// The Action returns a tea.Cmd, so buttons compose naturally with the
	// Bubble Tea command pipeline (here: emit a statusMsg, or tea.Quit).
	save := mnemonic.New("Save", 'S',
		func() tea.Cmd { return setStatus("Save triggered") },
		mnemonic.WithStyles(styles))
	load := mnemonic.New("Load", 'L',
		func() tea.Cmd { return setStatus("Load triggered") },
		mnemonic.WithStyles(styles))
	exit := mnemonic.New("Exit", 'E',
		func() tea.Cmd { return tea.Quit },
		mnemonic.WithStyles(styles))
	return model{buttons: []*mnemonic.Button{save, load, exit}}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.status = string(msg)
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		// Host-side key routing: ask every button whether the press
		// matches its binding, fire the first hit. The component does
		// not subscribe to messages itself, so this loop is required.
		for _, b := range m.buttons {
			if b.Matches(msg) {
				return m, b.Trigger()
			}
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	// Button.View renders one `[Label]` string with the mnemonic styled.
	// The host decides how buttons are arranged — here, joined on a row
	// with a two-space gap.
	parts := make([]string, len(m.buttons))
	for i, b := range m.buttons {
		parts[i] = b.View()
	}
	row := strings.Join(parts, "  ")
	title := lipgloss.NewStyle().Bold(true).Render("Mnemonic Button Example")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render("press s/l/e to trigger • q to quit")
	status := ""
	if m.status != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(m.status)
	}
	body := lipgloss.JoinVertical(lipgloss.Left, title, "", row, "", status, help)
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func main() {
	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
