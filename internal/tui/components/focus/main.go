//go:build ignore

// Example program demonstrating the focus.Handler component.
//
// Four widgets are registered, showing both built-in adaptation and the
// pattern for handling unsupported component types:
//
//   - a table (files)               — mnemonic '1', built-in *table.Model adapter
//   - a list (recent actions)       — mnemonic '4', wrapped in a custom
//     Focusable adapter because list.Model
//     lacks Focus/Blur entirely
//   - a textarea (description)      — mnemonic '2', built-in adapter
//   - a textinput (tags)            — mnemonic '3', built-in adapter
//
// The handler returns a *mnemonic.Button per registration; the host renders
// those buttons inside the panel border titles as the visible [N] indicator.
//
// Key routing in this example:
//
//  1. The host first asks the handler to interpret the message. If the
//     handler reports the message as handled (Tab, Shift+Tab, or a bound
//     alt+digit), the host does not forward it further — focus has already
//     shifted.
//  2. Otherwise the host forwards the message to the currently focused
//     widget (resolved via FocusedComponent so it does not depend on
//     insertion order).
//
// Run with: make example focus
package main

import (
	"fmt"
	"os"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

type model struct {
	handler    *focus.Handler
	files      *table.Model
	actions    *focusableList
	desc       *textarea.Model
	tags       *textinput.Model
	filesBtn   *mnemonic.Button
	actionsBtn *mnemonic.Button
	descBtn    *mnemonic.Button
	tagsBtn    *mnemonic.Button
}

// actionItem is a minimal list.DefaultItem so the list can render with the
// default delegate without a bespoke renderer.
type actionItem struct {
	title, desc string
}

func (a actionItem) Title() string       { return a.title }
func (a actionItem) Description() string { return a.desc }
func (a actionItem) FilterValue() string { return a.title }

// focusableList adapts a *list.Model to the focus.Focusable interface.
// list.Model has no native Focus/Blur, so the adapter toggles the list's
// KeyMap: while blurred, every binding is unbound so the widget ignores keys
// even if the host accidentally forwards them. Focus restores the saved
// KeyMap.
//
// This is the recommended pattern for any bubbles widget without Focus/Blur
// (viewport, list, custom tree-tables): wrap in a small struct that
// satisfies Focusable and routes via the host's FocusedComponent switch.
type focusableList struct {
	m     *list.Model
	saved list.KeyMap
}

func newFocusableList(m *list.Model) *focusableList {
	fl := &focusableList{m: m, saved: m.KeyMap}
	fl.m.KeyMap = list.KeyMap{}
	return fl
}

func (f *focusableList) Focus() tea.Cmd {
	f.m.KeyMap = f.saved
	return nil
}

func (f *focusableList) Blur() tea.Cmd {
	f.m.KeyMap = list.KeyMap{}
	return nil
}

func (f *focusableList) View() string { return f.m.View() }

func (f *focusableList) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := f.m.Update(msg)
	*f.m = updated
	return cmd
}

func newModel() model {
	cols := []table.Column{
		{Title: "Name", Width: 32},
		{Title: "Actions", Width: 18},
	}
	rows := []table.Row{
		{"SKILL.md", "<Edit> <Delete>"},
		{"scripts/git-commit.sh", "<Edit> <Delete>"},
		{"references/good-example.md", "<Edit> <Delete>"},
		{"references/bad-example.md", "<Edit> <Delete>"},
		{"assets/pr-template.md", "<Edit> <Delete>"},
	}
	files := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(8),
		table.WithWidth(54),
	)

	items := []list.Item{
		actionItem{title: "Rebuild manifest", desc: "regenerate asset.json"},
		actionItem{title: "Open in editor", desc: "launch $EDITOR on selected file"},
		actionItem{title: "Run linter", desc: "vet + gofmt across the asset"},
		actionItem{title: "Copy mnemonic", desc: "yank the asset id to clipboard"},
	}
	actions := list.New(items, list.NewDefaultDelegate(), 54, 8)
	actions.Title = "Actions"
	actions.SetShowStatusBar(false)
	actions.SetShowHelp(false)
	actions.SetFilteringEnabled(false)

	desc := textarea.New()
	desc.Placeholder = "A long description for this skill..."
	desc.SetWidth(40)
	desc.SetHeight(3)

	tags := textinput.New()
	tags.Placeholder = "git, review"
	tags.SetWidth(40)

	// Widgets stored as pointers in both the model and the handler so
	// updates flow through a single allocation. Otherwise the handler's
	// Focus/Blur calls would mutate a stale copy after the model received
	// a new value from Update.
	m := model{
		handler: focus.New(focus.WithModifier(focus.ModAlt)),
		files:   &files,
		actions: newFocusableList(&actions),
		desc:    &desc,
		tags:    &tags,
	}
	m.filesBtn = m.handler.AddMnemonic(m.files, '1')
	m.descBtn = m.handler.AddMnemonic(m.desc, '2')
	m.tagsBtn = m.handler.AddMnemonic(m.tags, '3')
	m.actionsBtn = m.handler.AddMnemonic(m.actions, '4')
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch kp.String() {
		case "ctrl+c":
			m.handler.Close()
			return m, tea.Quit
		}
	}

	if handled, cmd := m.handler.Update(msg); handled {
		return m, cmd
	}

	// Route message to whatever the handler currently considers focused.
	switch c := m.handler.FocusedComponent().(type) {
	case *table.Model:
		updated, cmd := c.Update(msg)
		*c = updated
		return m, cmd
	case *textarea.Model:
		updated, cmd := c.Update(msg)
		*c = updated
		return m, cmd
	case *textinput.Model:
		updated, cmd := c.Update(msg)
		*c = updated
		return m, cmd
	case *focusableList:
		return m, c.Update(msg)
	}
	return m, nil
}

func (m model) View() tea.View {
	panel := func(btn *mnemonic.Button, title, body string) string {
		head := lipgloss.JoinHorizontal(lipgloss.Top, btn.View(), " ",
			lipgloss.NewStyle().Bold(true).Render(title))
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Render(body)
		return lipgloss.JoinVertical(lipgloss.Left, head, box)
	}

	left := lipgloss.JoinVertical(lipgloss.Left,
		panel(m.filesBtn, "Files", m.files.View()),
		"",
		panel(m.actionsBtn, "Actions", m.actions.View()),
	)
	right := lipgloss.JoinVertical(lipgloss.Left,
		panel(m.descBtn, "Description", m.desc.View()),
		"",
		panel(m.tagsBtn, "Tags", m.tags.View()),
	)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render("tab / shift+tab cycle • alt+1..4 jump • ctrl+c quit")
	title := lipgloss.NewStyle().Bold(true).Render("Focus Handler Example")

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
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
