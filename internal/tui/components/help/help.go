// Package help provides a scrollable, markdown-rendered help dialog wired
// on top of [modal.Modal]. It loads `.md` files from [ManualRoot], renders
// them through Glamour, and drives a [viewport.Model] so the user can
// scroll vertically. Horizontal scrolling is disabled — text is wrapped to
// the viewport width so it always fits.
//
// The dialog accepts a [Request] identifying the topic name and the path
// to the file relative to [ManualRoot]. Path safety is enforced before any
// read happens: the extension must be `.md`, and the resolved path must
// stay inside [ManualRoot]. Anything else returns a typed error from
// `errors.go` that the dialog surfaces inside its own viewport.
package help

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/glamour"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// ManualRoot is the only folder the help dialog will read from. Paths in a
// [Request] are resolved relative to this directory and rejected if they
// escape it.
const ManualRoot = "docs/manual"

// Request names the help page to display.
type Request struct {
	// Topic is the human-facing label rendered in the dialog's title tab.
	Topic string
	// Path is the file path relative to [ManualRoot]. Must end in `.md`
	// and must not escape the root via `..`.
	Path string
}

type keymap struct {
	Close key.Binding
}

func defaultKeymap() keymap {
	return keymap{Close: key.NewBinding(key.WithKeys("esc", "q"))}
}

// content implements [modal.Content] for the help dialog. It owns the
// viewport that renders the loaded markdown and a small key map for
// closing the dialog.
type content struct {
	topic    string
	path     string
	width    int
	height   int
	viewport viewport.Model
	state    modal.LifecycleState
	keys     keymap
}

// New constructs a help [modal.Modal] of fixed size (width x height) that
// loads and renders req.Path from [ManualRoot]. The modal closes on `esc`
// or `q`. Construction never fails — load errors are caught and rendered
// inside the dialog's own viewport.
func New(id string, req Request, width, height int) *modal.Modal {
	c := &content{
		topic:  req.Topic,
		path:   req.Path,
		width:  width,
		height: height,
		keys:   defaultKeymap(),
	}

	innerW := width - 2  // rounded border
	innerH := height - 2 // rounded border
	// Layout heights inside the border:
	//   titleRow (bordered tab joined with a rule): 3
	//   viewport:                                   vH
	//   bottomRow (rule joined with bordered %):    3
	//   help footer:                                1
	vH := innerH - 7
	if vH < 1 {
		vH = 1
	}

	vp := viewport.New(viewport.WithWidth(innerW), viewport.WithHeight(vH))
	vp.SoftWrap = true

	body, err := loadManual(c.path, innerW)
	if err != nil {
		body = renderLoadError(err)
	}
	vp.SetContent(body)
	c.viewport = vp

	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	return modal.New(id, c, modal.WithStyle(style))
}

func (c *content) Init() tea.Cmd { return nil }

func (c *content) Update(msg tea.Msg) (modal.Content, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(kp, c.keys.Close) {
			c.state = modal.Cancelled
			return c, nil
		}
	}
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)
	return c, cmd
}

func (c *content) View() string {
	innerW := c.width - 2

	tabStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	tab := tabStyle.Render("Help: " + c.topic)
	tabW := lipgloss.Width(tab)
	titleRow := lipgloss.JoinHorizontal(
		lipgloss.Center,
		tab,
		strings.Repeat("─", max(0, innerW-tabW)),
	)

	pct := fmt.Sprintf("%3.0f%%", c.viewport.ScrollPercent()*100)
	pctBox := tabStyle.Render(pct)
	pctW := lipgloss.Width(pctBox)
	bottomRow := lipgloss.JoinHorizontal(
		lipgloss.Center,
		strings.Repeat("─", max(0, innerW-pctW)),
		pctBox,
	)

	help := lipgloss.NewStyle().
		Width(innerW).
		Foreground(lipgloss.Color("8")).
		Render("↑/k up • ↓/j down • esc/q close")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleRow,
		c.viewport.View(),
		bottomRow,
		help,
	)
}

func (c *content) Lifecycle() (modal.LifecycleState, any) {
	return c.state, nil
}

// loadManual validates rel, loads the file from [ManualRoot], and renders
// it through Glamour wrapped to width columns. All validation happens
// before the file system is touched so traversal attempts never become
// reads.
func loadManual(rel string, width int) (string, error) {
	if filepath.Ext(rel) != ".md" {
		return "", &InvalidExtensionError{Path: rel}
	}
	rootAbs, err := filepath.Abs(ManualRoot)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean("/" + strings.TrimPrefix(rel, "/"))
	fullAbs := filepath.Join(rootAbs, clean)
	check, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) {
		return "", &OutsideRootError{Path: rel}
	}
	data, err := os.ReadFile(fullAbs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", &NotFoundError{Path: rel}
		}
		return "", err
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", &RenderError{Path: rel, Err: err}
	}
	out, err := r.Render(string(data))
	if err != nil {
		return "", &RenderError{Path: rel, Err: err}
	}
	return out, nil
}

func renderLoadError(err error) string {
	return fmt.Sprintf("\n  Failed to load manual:\n\n  %s\n", err.Error())
}
