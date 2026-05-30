//go:build ignore

// Example program demonstrating the modal.Modal component with a confirm
// dialog.
//
// Run with: make example modal
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

type model struct {
	modal  *modal.Modal
	status string
	width  int
	height int
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case modal.ResolvedMsg:
		if msg.Confirmed {
			m.status = fmt.Sprintf("%s: confirmed", msg.ID)
		} else {
			m.status = fmt.Sprintf("%s: cancelled", msg.ID)
		}
		m.modal = nil
		return m, nil
	}

	if m.modal != nil {
		var cmd tea.Cmd
		m.modal, cmd = m.modal.Update(msg)
		return m, cmd
	}

	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch kp.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "o":
			c := modal.NewConfirm("demo", "Proceed with the operation?", nil)
			m.modal = c
			return m, c.Init()
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	body := m.body()
	v := tea.NewView(body)
	v.AltScreen = true
	if m.modal != nil {
		v.SetContent(m.modal.Render(body, m.width, m.height))
	}
	return v
}

func (m model) body() string {
	title := lipgloss.NewStyle().Bold(true).Render("Modal Example")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render("o open confirm • y/n in modal • q quit")
	status := ""
	if m.status != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(m.status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, "", status, help)
}

func main() {
	p := tea.NewProgram(model{})
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
