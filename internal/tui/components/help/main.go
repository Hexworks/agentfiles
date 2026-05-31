//go:build ignore

// Example program demonstrating the help.Modal component.
//
// The dialog loads `.md` files from [help.ManualRoot] (relative to the
// process working directory) and renders them through Glamour inside a
// fixed-size [modal.Modal]. The viewport scrolls vertically; horizontal
// scroll is disabled because content is wrapped to the dialog width.
//
// Run from the repo root so the relative manual root resolves:
//
//	make example help
//
// Press `?` to open the help dialog, `esc`/`q` inside the dialog to close,
// `q`/`ctrl+c` from the main view to quit.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

const (
	helpDialogWidth  = 80
	helpDialogHeight = 24
)

// helpPage is one entry in the example's topic list. A real host would
// derive these from context (focused screen, hovered item, etc.).
type helpPage struct {
	Topic string
	Path  string
}

var pages = []helpPage{
	{Topic: "Something", Path: "xul/something.md"},
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
		m.status = fmt.Sprintf("closed: %s", msg.ID)
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
		case "?":
			page := pages[0]
			h := help.New("help-"+page.Topic, help.Request{
				Topic: page.Topic,
				Path:  page.Path,
			}, helpDialogWidth, helpDialogHeight)
			m.modal = h
			return m, h.Init()
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
	title := lipgloss.NewStyle().Bold(true).Render("Help Dialog Example")
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render("? open help • q quit")
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
