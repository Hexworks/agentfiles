package main

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

type row struct {
	id   string
	name string
}

var demoRows = []row{
	{"#1", "Alpha"},
	{"#2", "Bravo"},
	{"#3", "Charlie"},
	{"#4", "Delta"},
	{"#5", "Echo"},
	{"#6", "Foxtrot"},
	{"#7", "Golf"},
}

const (
	colIDWidth      = 6
	colNameWidth    = 14
	colActionsWidth = 30
)

// Screen layout offsets used to anchor the confirmation modal directly
// underneath the activating button. The table is the first widget after the
// title, which is one printed line plus one line of bottom padding.
const (
	// titleHeight = title text (1 line) + bottom padding (1 line).
	titleHeight = 2
	// firstRowY = title + table header.
	firstRowY = titleHeight + 1
	// actionsCellStartX = cumulative width of the ID and Name cells
	// (col.Width + 2 cell-padding chars each) plus the actions cell's own
	// left padding (1 char).
	actionsCellStartX = (colIDWidth + 2) + (colNameWidth + 2) + 1
)

type model struct {
	table   table.Model
	rows    []row
	buttons []*mnemonic.Button
	modal   *modal.Modal
	width   int
	height  int
	status  string
}

func newModel() model {
	cols := []table.Column{
		{Title: "ID", Width: colIDWidth},
		{Title: "Name", Width: colNameWidth},
		{Title: "Actions", Width: colActionsWidth},
	}

	tableWidth := colIDWidth + colNameWidth + colActionsWidth + 6
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithWidth(tableWidth),
		table.WithHeight(len(demoRows)+2),
	)
	selectedColor := lipgloss.Color("13") // pink
	mnemonicColor := lipgloss.Color("99") // purple

	styles := table.DefaultStyles()
	styles.Selected = lipgloss.NewStyle().Bold(true).Foreground(selectedColor)
	t.SetStyles(styles)

	btnStyles := mnemonic.Styles{
		Bracket:  lipgloss.NewStyle().Bold(true).Foreground(selectedColor),
		Label:    lipgloss.NewStyle().Bold(true).Foreground(selectedColor),
		Mnemonic: lipgloss.NewStyle().Bold(true).Underline(true).Foreground(mnemonicColor),
	}

	m := model{
		table: t,
		rows:  demoRows,
	}
	viewBtn := mnemonic.New("View", 'V', func() tea.Cmd {
		return openConfirm("confirm-view", "View selected item?", 0)
	}, mnemonic.WithStyles(btnStyles))
	// +1 accounts for the literal space joiner between buttons.
	viewWidth := lipgloss.Width(viewBtn.View()) + 1
	delBtn := mnemonic.New("Delete", 'D', func() tea.Cmd {
		return openConfirm("confirm-delete", "Delete selected item?", viewWidth)
	}, mnemonic.WithStyles(btnStyles))
	m.buttons = []*mnemonic.Button{viewBtn, delBtn}
	m.refreshRows()
	return m
}

func (m *model) refreshRows() {
	out := make([]table.Row, len(m.rows))
	cursor := m.table.Cursor()
	for i, r := range m.rows {
		actions := ""
		if i == cursor {
			actions = m.renderButtons()
		}
		out[i] = table.Row{r.id, r.name, actions}
	}
	m.table.SetRows(out)
}

func (m *model) renderButtons() string {
	parts := make([]string, len(m.buttons))
	for i, b := range m.buttons {
		parts[i] = b.View()
	}
	return strings.Join(parts, " ")
}

type openConfirmMsg struct {
	id      string
	prompt  string
	offsetX int // X offset of the activating button within the actions cell
}

func openConfirm(id, prompt string, offsetX int) tea.Cmd {
	return func() tea.Msg {
		return openConfirmMsg{id: id, prompt: prompt, offsetX: offsetX}
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width)
	case openConfirmMsg:
		x := actionsCellStartX + msg.offsetX
		y := firstRowY + m.table.Cursor() + 1
		c := modal.NewConfirm(msg.id, msg.prompt, []modal.Option{modal.WithAnchor(x, y)})
		m.modal = c
		return m, c.Init()
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
		if kp.String() == "ctrl+c" || kp.String() == "q" {
			return m, tea.Quit
		}
		for _, b := range m.buttons {
			if b.Matches(kp) {
				return m, b.Trigger()
			}
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	m.refreshRows()
	return m, cmd
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
	title := lipgloss.NewStyle().Bold(true).Padding(0, 0, 1, 0).Render("Mnemonic Table Playground")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"↑/k up • ↓/j down • v view • d delete • q quit",
	)
	status := ""
	if m.status != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(m.status) + "\n"
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, m.table.View(), "", status+help)
}

func main() {
	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
