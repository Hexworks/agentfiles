package diffview

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

type keymap struct {
	Close key.Binding
}

func defaultKeymap() keymap {
	return keymap{Close: key.NewBinding(key.WithKeys("esc"))}
}

// content implements [modal.Content] for the diff dialog. It mirrors the help
// dialog's viewport-backed frame: a scrollable body plus a small key map for
// closing. The body text is pre-built (see [BuildDiff] / [ErrorText]) so the
// modal never touches the domain — it only scrolls and closes.
type content struct {
	title    string
	body     string
	width    int
	height   int
	viewport viewport.Model
	state    modal.LifecycleState
	keys     keymap
}

// New constructs a diff [modal.Modal] sized for (width x height) showing the
// pre-built body text under the given title (typically the file path). The
// modal closes on `esc`; `q` is reserved for the shell-level quit binding so
// the user can exit the application without first closing the dialog.
// Construction never fails — the caller has already turned bytes (or an error)
// into a display string. Subsequent [Modal.SetSize] calls reflow the viewport.
func New(id, title, body string, width, height int) *modal.Modal {
	c := &content{
		title: title,
		body:  body,
		keys:  defaultKeymap(),
	}
	innerW, vH := diffInner(width, height)
	vp := viewport.New(viewport.WithWidth(innerW), viewport.WithHeight(vH))
	vp.SoftWrap = true
	c.viewport = vp
	c.applySize(width, height, innerW, vH)
	return modal.New(id, c, modal.WithStyle(styles.HelpModalStyle))
}

// ErrorText renders a diff-load failure into the body shown inside the modal
// frame, so a DiffLocalReadError (or any diff failure) surfaces in the same
// viewport rather than as a toast or a blank pane.
func ErrorText(err error) string {
	return fmt.Sprintf("Failed to produce diff:\n\n%s", err.Error())
}

// diffInner translates the outer modal dimensions into the inner viewport
// width and height, deducting the rounded border (2 each axis) and the four
// chrome rows inside the border: title row (3) + bottom row (3) + help footer
// (1) = 7. Mirrors help.helpInner so the two dialogs frame identically. Both
// axes clamp at 1 so the viewport stays valid on very narrow terminals.
func diffInner(width, height int) (int, int) {
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	vH := height - 2 - 7
	if vH < 1 {
		vH = 1
	}
	return innerW, vH
}

// applySize updates the cached width/height, resizes the viewport, and re-sets
// the body content. Shared between New and SetSize so the resize path always
// touches the same fields.
func (c *content) applySize(width, height, innerW, vH int) {
	c.width = width
	c.height = height
	c.viewport.SetWidth(innerW)
	c.viewport.SetHeight(vH)
	c.viewport.SetContent(c.body)
}

// SetSize re-runs the diff layout for new outer dimensions.
func (c *content) SetSize(width, height int) {
	innerW, vH := diffInner(width, height)
	c.applySize(width, height, innerW, vH)
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

	tab := styles.HelpTabStyle.Render("Diff: " + c.title)
	tabW := lipgloss.Width(tab)
	titleRow := lipgloss.JoinHorizontal(
		lipgloss.Center,
		tab,
		strings.Repeat("─", max(0, innerW-tabW)),
	)

	pct := fmt.Sprintf("%3.0f%%", c.viewport.ScrollPercent()*100)
	pctBox := styles.HelpTabStyle.Render(pct)
	pctW := lipgloss.Width(pctBox)
	bottomRow := lipgloss.JoinHorizontal(
		lipgloss.Center,
		strings.Repeat("─", max(0, innerW-pctW)),
		pctBox,
	)

	help := styles.HelpHintStyle.Width(innerW).Render("↑/k up • ↓/j down • esc close")

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
