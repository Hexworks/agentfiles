package treetable

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

func sampleTree() *Node {
	return &Node{
		Label: "root/",
		Children: []*Node{
			{Label: "a.md"},
			{
				Label: "sub/",
				Children: []*Node{
					{Label: "b.go"},
					{Label: "c.go"},
				},
			},
			{Label: "z.md"},
		},
	}
}

// flatten DFS order must line up 1:1 with the lipgloss tree output. The
// invariant lets the row builder pair a node with its prefixed label by
// index — losing it would silently misroute actions to the wrong node.
func TestFlattenAndTreeLinesAlign(t *testing.T) {
	root := sampleTree()
	flat := flatten(root)
	lines := renderTreeLines(root)
	if len(flat) != len(lines) {
		t.Fatalf("flat=%d lines=%d want equal", len(flat), len(lines))
	}
	for i, n := range flat {
		if !strings.Contains(lines[i], n.Label) {
			t.Fatalf("line %d %q missing label %q", i, lines[i], n.Label)
		}
	}
}

func TestSelectedNodeFollowsCursor(t *testing.T) {
	m := New(WithRoot(sampleTree()))
	m.Focus()
	got := m.SelectedNode()
	if got == nil || got.Label != "root/" {
		t.Fatalf("initial selection: got %v", got)
	}
	m.table.MoveDown(2)
	m.refreshRows()
	got = m.SelectedNode()
	if got == nil || got.Label != "sub/" {
		t.Fatalf("after MoveDown(2): got %v", got)
	}
}

// ActionsFunc fires only for the cursor row. Non-cursor rows must render an
// empty actions cell and contribute nothing to Buttons().
func TestActionsScopedToCursor(t *testing.T) {
	var actionsCalls []string
	fn := func(n *Node) []*mnemonic.Button {
		actionsCalls = append(actionsCalls, n.Label)
		return []*mnemonic.Button{
			mnemonic.New("Delete", 'D', func() tea.Cmd { return nil }),
		}
	}
	m := New(
		WithRoot(sampleTree()),
		WithActions(Column{Title: "Actions", Width: 18}, fn),
	)
	m.Focus()
	actionsCalls = nil
	m.refreshRows()
	if len(actionsCalls) != 1 || actionsCalls[0] != "root/" {
		t.Fatalf("expected single call for cursor row, got %v", actionsCalls)
	}
	if len(m.Buttons()) != 1 {
		t.Fatalf("expected 1 cursor button, got %d", len(m.Buttons()))
	}
}

// Update routes a matching key press to the cursor row's button before
// touching navigation. Otherwise navigation keys would shadow shortcuts that
// share their character with a movement binding.
func TestUpdateTriggersCurrentRowButton(t *testing.T) {
	triggered := 0
	fn := func(n *Node) []*mnemonic.Button {
		return []*mnemonic.Button{
			mnemonic.New("Delete", 'D', func() tea.Cmd {
				triggered++
				return nil
			}),
		}
	}
	m := New(
		WithRoot(sampleTree()),
		WithActions(Column{Title: "Actions", Width: 18}, fn),
	)
	m.Focus()
	m.refreshRows()
	_, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if triggered != 1 {
		t.Fatalf("expected button trigger, got %d", triggered)
	}
}

// View must include the caption when a title is set; otherwise the panel
// frame would render with empty top decoration and look broken.
func TestViewIncludesTitle(t *testing.T) {
	m := New(WithRoot(sampleTree()), WithTitle("Files"))
	out := m.View()
	if !strings.Contains(out, "Files") {
		t.Fatalf("view missing title: %q", out)
	}
	if !strings.Contains(out, "┌") || !strings.Contains(out, "└") {
		t.Fatalf("view missing panel frame: %q", out)
	}
}
