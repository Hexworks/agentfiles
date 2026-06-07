package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/editor"
)

const editPath = "docs/manual/"

// viewContent is a modal.Content with one mnemonic button: `v` opens the
// system editor on editFilePath. ESC cancels the modal. The content stays
// Active across the editor invocation so the modal remains open when the
// editor returns.
type viewContent struct {
	state    modal.ResolutionState
	lastErr  error
	editedAt int
}

func (c *viewContent) Init() tea.Cmd { return nil }

func (c *viewContent) Update(msg tea.Msg) (modal.Content, tea.Cmd) {
	switch m := msg.(type) {
	case editor.FinishedMsg:
		c.lastErr = m.Err
		c.editedAt++
		return c, nil
	case tea.KeyPressMsg:
		switch m.String() {
		case "v", "V":
			return c, editor.Open(editPath)
		case "esc":
			c.state = modal.Cancelled
			return c, nil
		}
	}
	return c, nil
}

func (c *viewContent) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("Action")
	button := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 2).
		Render("[v] View")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render("press v to edit docs/manual/ • esc to close")
	status := ""
	if c.editedAt > 0 {
		s := fmt.Sprintf("editor returned %d time(s)", c.editedAt)
		if c.lastErr != nil {
			s = fmt.Sprintf("editor error: %v", c.lastErr)
		}
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, "", button, "", status, hint)
}

func (c *viewContent) Resolution() (modal.ResolutionState, any) {
	return c.state, nil
}

type model struct {
	width  int
	height int
	modal  *modal.Modal
	status string
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case modal.ResolvedMsg:
		m.status = fmt.Sprintf("modal closed: %s", msg.ID)
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
			mdl := modal.New("view-modal", &viewContent{})
			m.modal = mdl
			return m, mdl.Init()
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
	title := lipgloss.NewStyle().Bold(true).Render("Editor-Resume Playground")
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render("press o to open modal • q to quit")
	status := ""
	if m.status != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(m.status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, "", status, prompt)
}

func main() {
	p := tea.NewProgram(model{})
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
