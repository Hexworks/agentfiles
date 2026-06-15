package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// backOnlyScreenBase carries the wiring every "Coming soon" navigation stub
// shares: a [Back] button bound to `b` + `esc`, viewport tracking from
// WindowSizeMsg, and a StatusKeys + body helper. Each stub embeds it and
// supplies only its title plus the one-line body sentence.
type backOnlyScreenBase struct {
	back          *mnemonic.Button
	width, height int
}

func newBackOnlyBase() backOnlyScreenBase {
	return backOnlyScreenBase{
		back: mnemonic.New(
			"Back",
			'b',
			func() tea.Cmd { return popCmd() },
			mnemonic.WithExtraBindingKeys("esc"),
		),
	}
}

// handleMsg returns (cmd, true) when the base consumed the message
// (back-key trigger or window-size update). Callers fall through with
// (nil, false) when the message is not theirs.
func (b *backOnlyScreenBase) handleMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = m.Width
		b.height = m.Height
		return nil, true
	case tea.KeyPressMsg:
		if b.back.Matches(m) {
			return b.back.Trigger(), true
		}
	}
	return nil, false
}

// statusKeys returns the single [Back] binding. Stubs expose [Back] in the
// status bar because the body has no other cue, mirroring the Settings
// screen's explicit exception.
func (b *backOnlyScreenBase) statusKeys() []key.Binding {
	return []key.Binding{b.back.Binding()}
}

// renderBody renders sentence + spacer + left-aligned [Back] at its
// natural height. The shell stacks the body, toast, and status bar
// without padding so the body takes only the room it needs.
func (b *backOnlyScreenBase) renderBody(width int, sentence string) string {
	backRow := " " + b.back.View()
	return lipgloss.JoinVertical(lipgloss.Left, sentence, "", backRow)
}
