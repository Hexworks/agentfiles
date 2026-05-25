// Command af is the agentfiles CLI. It parses the --registry flag and drops
// the user into the TUI defined in internal/tui, which is the only interface
// agentfiles exposes.
//
// TEMP (task 0015): main currently boots a standalone Profiles screen used
// to validate the new alt-screen UI from tasks/current/0015_task_refactor_ui.
// Restore the original tui.Run(app.New(*registryPath)) call once the screen
// is wired into the broader navigation flow.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/registry"
)

// tableBorderStyle wraps the table viewport (header + rows) in a normal-border
// frame, matching the upstream bubbles/table example. The dim grey (240) keeps
// the border quiet so the cyan title and selected-row highlight stand out.
var tableBorderStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

func main() {
	registryPath := flag.String("registry", registry.DefaultPath(), "path to profile registry")
	flag.Parse()
	service := app.New(*registryPath)
	if _, err := tea.NewProgram(newProfilesModel(service)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// profilesLoadedMsg carries the result of the asynchronous profile load that
// fires from Init.
type profilesLoadedMsg struct {
	profiles []registry.ProfileRef
	err      error
}

type profilesModel struct {
	service *app.Service
	table   table.Model
	err     error
	width   int
	height  int
}

func newProfilesModel(service *app.Service) *profilesModel {
	t := table.New(
		table.WithColumns(profileColumns(80)),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	t.SetStyles(profileTableStyles())
	return &profilesModel{service: service, table: t}
}

// profileTableStyles mirrors the upstream bubbles/table example: a bottom-
// border on the header row that doubles as a separator between header and
// body, and a highlighted selected row. The header is left un-bolded so the
// border (not the weight) carries the visual hierarchy.
func profileTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	return s
}

func (m *profilesModel) Init() tea.Cmd {
	return loadProfilesCmd(m.service)
}

func loadProfilesCmd(s *app.Service) tea.Cmd {
	return func() tea.Msg {
		profiles, err := s.ListProfiles()
		if err != nil {
			return profilesLoadedMsg{err: err}
		}
		return profilesLoadedMsg{profiles: profiles}
	}
}

func (m *profilesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case profilesLoadedMsg:
		m.err = msg.err
		m.table.SetRows(profileRows(msg.profiles))
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Inner width = terminal width minus the two border cells the
		// tableBorderStyle adds. Without this subtraction the table renders
		// one cell wider than its frame and the right border drops off.
		inner := msg.Width - 2
		if inner < 20 {
			inner = 20
		}
		m.table.SetColumns(profileColumns(inner))
		m.table.SetWidth(inner)
		// Reserve rows for the title block (3 lines incl. border),
		// surrounding blank lines (2), the table border (2), and the
		// status bar (1).
		tableHeight := msg.Height - 8
		if tableHeight < 3 {
			tableHeight = 3
		}
		m.table.SetHeight(tableHeight)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *profilesModel) View() tea.View {
	title := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("14")).
		Padding(0, 1).
		Render("Profiles")

	var body string
	switch {
	case m.err != nil:
		body = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Render(fmt.Sprintf("failed to load profiles: %v", m.err))
	case len(m.table.Rows()) == 0:
		body = tableBorderStyle.Render(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Padding(0, 1).
				Render("no profiles registered"),
		)
	default:
		body = tableBorderStyle.Render(m.table.View())
	}

	status := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render("↑/k up • ↓/j down • q quit")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", status)

	var v tea.View
	v.SetContent(content)
	v.AltScreen = true
	v.WindowTitle = "agentfiles — profiles"
	return v
}

// profileColumns returns table columns sized to the available inner width
// (terminal width minus the wrapping border). Path absorbs leftover space;
// a floor keeps the column usable on narrow terminals.
func profileColumns(innerWidth int) []table.Column {
	const (
		idWidth         = 16
		nameWidth       = 20
		lastOpenedWidth = 20
		minPathWidth    = 16
	)
	pathWidth := innerWidth - idWidth - nameWidth - lastOpenedWidth
	if pathWidth < minPathWidth {
		pathWidth = minPathWidth
	}
	return []table.Column{
		{Title: "ID", Width: idWidth},
		{Title: "Name", Width: nameWidth},
		{Title: "Path", Width: pathWidth},
		{Title: "Last opened", Width: lastOpenedWidth},
	}
}

func profileRows(profiles []registry.ProfileRef) []table.Row {
	rows := make([]table.Row, 0, len(profiles))
	for _, p := range profiles {
		rows = append(rows, table.Row{
			p.ID,
			p.Name,
			p.Path,
			formatLastOpened(p.LastOpenedAt),
		})
	}
	return rows
}

func formatLastOpened(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}
