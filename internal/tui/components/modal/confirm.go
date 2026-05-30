package modal

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// confirmContent implements [Content] for a binary Yes / No prompt. The
// prompt body is fixed at construction time so the modal can be reused
// without rebuilding its state machine.
type confirmContent struct {
	prompt   string
	selected confirmChoice
	state    ResolutionState
	styles   ConfirmStyles
	keys     confirmKeys
}

type confirmChoice int

const (
	confirmYes confirmChoice = iota
	confirmNo
)

// ConfirmStyles themes the confirmation dialog.
type ConfirmStyles struct {
	Prompt   lipgloss.Style
	Button   lipgloss.Style
	Selected lipgloss.Style
}

// DefaultConfirmStyles returns palette-neutral defaults; callers that want
// themed buttons pass [WithConfirmStyles].
func DefaultConfirmStyles() ConfirmStyles {
	return ConfirmStyles{
		Prompt:   lipgloss.NewStyle().Padding(0, 0, 1, 0),
		Button:   lipgloss.NewStyle().Padding(0, 2).Border(lipgloss.RoundedBorder()),
		Selected: lipgloss.NewStyle().Padding(0, 2).Border(lipgloss.RoundedBorder()).Bold(true).Reverse(true),
	}
}

type confirmKeys struct {
	Left    key.Binding
	Right   key.Binding
	Yes     key.Binding
	No      key.Binding
	Confirm key.Binding
	Cancel  key.Binding
}

func defaultConfirmKeys() confirmKeys {
	return confirmKeys{
		Left:    key.NewBinding(key.WithKeys("left", "h", "shift+tab")),
		Right:   key.NewBinding(key.WithKeys("right", "l", "tab")),
		Yes:     key.NewBinding(key.WithKeys("y", "Y")),
		No:      key.NewBinding(key.WithKeys("n", "N")),
		Confirm: key.NewBinding(key.WithKeys("enter")),
		Cancel:  key.NewBinding(key.WithKeys("esc")),
	}
}

// ConfirmOption configures a [NewConfirm] modal beyond the base [Option]s.
type ConfirmOption func(*confirmContent)

// WithConfirmStyles overrides the default confirmation palette.
func WithConfirmStyles(s ConfirmStyles) ConfirmOption {
	return func(c *confirmContent) { c.styles = s }
}

// NewConfirm constructs a Yes / No modal whose payload is a bool. On
// confirmation the [ResolvedMsg].Value is true; on cancellation the modal
// follows the standard [Cancelled] contract (Confirmed=false, Value=nil).
//
// prompt is rendered verbatim above the buttons. Callers typically pass a
// sentence like "Delete profile 'staging'?" — the dialog does not add a
// question mark of its own.
//
// Base [Option]s (e.g. [WithStyle], [WithZ]) flow through opts; confirmation-
// specific tweaks use [ConfirmOption]s passed through confirmOpts.
func NewConfirm(id, prompt string, opts []Option, confirmOpts ...ConfirmOption) *Modal {
	c := &confirmContent{
		prompt:   prompt,
		selected: confirmYes,
		styles:   DefaultConfirmStyles(),
		keys:     defaultConfirmKeys(),
	}
	for _, opt := range confirmOpts {
		opt(c)
	}
	return New(id, c, opts...)
}

func (c *confirmContent) Init() tea.Cmd { return nil }

func (c *confirmContent) Update(msg tea.Msg) (Content, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch {
	case key.Matches(keyMsg, c.keys.Left):
		c.selected = confirmYes
	case key.Matches(keyMsg, c.keys.Right):
		c.selected = confirmNo
	case key.Matches(keyMsg, c.keys.Yes):
		c.selected = confirmYes
		c.state = Confirmed
	case key.Matches(keyMsg, c.keys.No):
		c.selected = confirmNo
		c.state = Cancelled
	case key.Matches(keyMsg, c.keys.Confirm):
		if c.selected == confirmYes {
			c.state = Confirmed
		} else {
			c.state = Cancelled
		}
	case key.Matches(keyMsg, c.keys.Cancel):
		c.state = Cancelled
	}
	return c, nil
}

func (c *confirmContent) View() string {
	yesStyle, noStyle := c.styles.Button, c.styles.Button
	if c.selected == confirmYes {
		yesStyle = c.styles.Selected
	} else {
		noStyle = c.styles.Selected
	}
	buttons := lipgloss.JoinHorizontal(
		lipgloss.Top,
		yesStyle.Render("Yes"),
		"   ",
		noStyle.Render("No"),
	)
	var sb strings.Builder
	sb.WriteString(c.styles.Prompt.Render(c.prompt))
	sb.WriteString("\n")
	sb.WriteString(buttons)
	return sb.String()
}

func (c *confirmContent) Resolution() (ResolutionState, any) {
	if c.state == Confirmed {
		return Confirmed, true
	}
	return c.state, nil
}
