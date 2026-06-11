package shell

import (
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// renderStatusBar joins the global key hints with the active screen's
// dynamic mnemonic bindings into a single line. Order is fixed: nav
// keys first, then global mnemonics, then the screen's dynamic
// bindings. Screen-level labelled buttons (e.g. `[Create]`) are
// deliberately not represented here — the rule is that they appear on
// the screen body and must not be duplicated in the bar.
func renderStatusBar(global globalKeyMap, dynamic []key.Binding) string {
	bindings := []key.Binding{
		global.Up,
		global.Down,
		global.Notifications,
		global.Settings,
		global.Help,
		global.Quit,
	}
	bindings = append(bindings, dynamic...)

	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		if hint := keyHint(b); hint != "" {
			parts = append(parts, hint)
		}
	}
	return strings.Join(parts, "  ")
}

func keyHint(b key.Binding) string {
	h := b.Help()
	if h.Key == "" {
		return ""
	}
	return styles.MutedStyle.Render(h.Key + " " + h.Desc)
}
