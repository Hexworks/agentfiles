//go:build ignore

// Example program demonstrating the treetable.Model component.
//
// The example renders a skill directory layout as a tree-table with
// context-aware mnemonic action buttons:
//
//   - directories expose [Delete] only
//   - files expose [Edit] [Delete]
//
// A focus.Handler registers the tree-table so Tab / Shift+Tab can rotate
// focus. Confirmation modals open below the activating button, mirroring
// the pattern from the mnemonic playground.
//
// Run with: go run ./internal/tui/components/treetable/main.go
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
)

// fileKind discriminates the row payload so the action func knows which
// buttons to render. Directories get [Delete] only; files get [Edit] [Delete].
type fileKind int

const (
	kindDir fileKind = iota
	kindFile
)

type fileData struct {
	kind fileKind
	path string
}

type model struct {
	handler *focus.Handler
	tt      *treetable.Model
	modal   *modal.Modal
	width   int
	height  int
	status  string
}

func newModel() model {
	root := &treetable.Node{
		Label: "review-task/",
		Data:  fileData{kind: kindDir, path: "review-task"},
		Children: []*treetable.Node{
			{Label: "SKILL.md", Data: fileData{kind: kindFile, path: "review-task/SKILL.md"}},
			{
				Label: "scripts/",
				Data:  fileData{kind: kindDir, path: "review-task/scripts"},
				Children: []*treetable.Node{
					{Label: "git-commit.sh", Data: fileData{kind: kindFile, path: "review-task/scripts/git-commit.sh"}},
				},
			},
			{
				Label: "references/",
				Data:  fileData{kind: kindDir, path: "review-task/references"},
				Children: []*treetable.Node{
					{Label: "good-example.md", Data: fileData{kind: kindFile, path: "review-task/references/good-example.md"}},
					{Label: "bad-example.md", Data: fileData{kind: kindFile, path: "review-task/references/bad-example.md"}},
				},
			},
			{
				Label: "assets/",
				Data:  fileData{kind: kindDir, path: "review-task/assets"},
				Children: []*treetable.Node{
					{Label: "pr-template.md", Data: fileData{kind: kindFile, path: "review-task/assets/pr-template.md"}},
				},
			},
		},
	}

	btnStyles := mnemonic.ThemedStyles(
		ansi.Red,           // accent for the brackets
		ansi.BrightMagenta, // mnemonic letter (bold + underlined)
		ansi.BrightMagenta, // remainder of the label
	)

	// Action func: directories expose Delete only; files expose Edit + Delete.
	// New buttons every render — they are cheap and capture the current node.
	actionsFn := func(n *treetable.Node) []*mnemonic.Button {
		d, _ := n.Data.(fileData)
		path := d.path
		del := mnemonic.New("Delete", 'D', func() tea.Cmd {
			return openConfirm("delete:"+path, "Delete "+path+"?")
		}, mnemonic.WithStyles(btnStyles))
		if d.kind == kindDir {
			return []*mnemonic.Button{del}
		}
		edit := mnemonic.New("Edit", 'E', func() tea.Cmd {
			return openConfirm("edit:"+path, "Edit "+path+"?")
		}, mnemonic.WithStyles(btnStyles))
		return []*mnemonic.Button{edit, del}
	}

	tt := treetable.New(
		treetable.WithRoot(root),
		treetable.WithNameColumn(treetable.Column{Title: "Name", Width: 32}),
		treetable.WithActions(treetable.Column{Title: "Actions", Width: 18}, actionsFn),
		treetable.WithTitle("Files"),
		treetable.WithHeight(10),
	)

	m := model{
		handler: focus.New(),
		tt:      tt,
	}
	m.handler.Add(m.tt)
	return m
}

type openConfirmMsg struct {
	id     string
	prompt string
}

func openConfirm(id, prompt string) tea.Cmd {
	return func() tea.Msg {
		return openConfirmMsg{id: id, prompt: prompt}
	}
}

func (m model) Init() tea.Cmd { return m.handler.FocusIndex(0) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case openConfirmMsg:
		c := modal.NewConfirm(msg.id, msg.prompt, nil)
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
			m.handler.Close()
			return m, tea.Quit
		}
	}

	if handled, cmd := m.handler.Update(msg); handled {
		return m, cmd
	}

	if _, ok := m.handler.FocusedComponent().(*treetable.Model); ok {
		var cmd tea.Cmd
		m.tt, cmd = m.tt.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() tea.View {
	title := lipgloss.NewStyle().Bold(true).Padding(0, 0, 1, 0).Render("Tree-Table Example")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"↑/k up • ↓/j down • e edit • d delete • tab focus • q quit",
	)
	status := ""
	if m.status != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(m.status) + "\n"
	}
	body := lipgloss.JoinVertical(lipgloss.Left, title, m.tt.View(), "", status+help)
	v := tea.NewView(body)
	v.AltScreen = true
	if m.modal != nil {
		v.SetContent(m.modal.Render(body, m.width, m.height))
	}
	return v
}

func main() {
	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
