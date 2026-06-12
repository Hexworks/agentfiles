// Package shell implements the root Bubble Tea program for agentfiles.
// It runs in alt-screen mode, owns a screen-router stack, intercepts
// the global key set (n notifications, s settings, ? info, q / ctrl+c
// quit), hosts the notifications Log + Toast widget mounted above
// the status bar, and delegates everything else to the active
// (top-of-stack) Screen.
//
// Screens implement the [Screen] interface. They push or pop other
// screens by emitting [PushScreenMsg] / [PopScreenMsg] from their
// Update; the root model is the only place the stack mutates. The
// shell seeds the root with the Welcome screen; entity screens
// (Edit Profile, Edit Asset, …) land in later tasks (0025–0029) and
// are reached through Welcome's menu or screen-local navigation.
package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Screen is the unit of navigation on the shell's stack.
//
// Body receives only the available width. Screens render at their
// natural height — the shell stacks title, body, toast, and status bar
// without padding the body to fill the terminal.
//
// StatusKeys returns the dynamic bindings the status bar appends to
// the global set — typically the focused row's mnemonic-button
// bindings. The screen's labelled buttons (e.g. `[Create]`) must not
// appear here: they are already visible on the screen body and must
// not be duplicated in the bar.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	Body(width int) string
	Title() string
	StatusKeys() []key.Binding
}

// PushScreenMsg pushes Screen onto the shell's stack. The shell runs
// the new screen's Init after the push.
type PushScreenMsg struct{ Screen Screen }

// PopScreenMsg removes the top screen. On a single-screen stack it is
// a no-op (the root screen cannot be popped).
type PopScreenMsg struct{}

// pushCmd returns a tea.Cmd that, when run, emits a PushScreenMsg for
// s. Lives next to the message type so authors searching for "how do I
// push a screen?" find the constructor and the message together.
func pushCmd(s Screen) tea.Cmd {
	return func() tea.Msg { return PushScreenMsg{Screen: s} }
}

// popCmd returns a tea.Cmd that, when run, emits a PopScreenMsg.
func popCmd() tea.Cmd {
	return func() tea.Msg { return PopScreenMsg{} }
}

// modalMinWidth / modalMinHeight is the floor the shell guarantees a
// modal opens at, even before the first WindowSizeMsg has arrived
// (when shell.Model.width/height are zero). bubbles/table degrades on
// non-positive dimensions; the floor keeps the first keystroke safe.
const (
	modalMinWidth  = 40
	modalMinHeight = 10
	// chromeHeight is the rows the shell reserves outside any modal:
	// the title bar (3) plus the status bar (1). Kept in sync with
	// the layout in shell.go::View.
	chromeHeight = 4
)

// modalSize returns the (width, height) the shell hands to a modal
// constructor: the current viewport minus the chrome reservation,
// floored to (modalMinWidth, modalMinHeight) so a pre-WindowSize open
// still feeds a usable rectangle to bubbles/table.
func modalSize(width, height int) (int, int) {
	w := width
	if w < modalMinWidth {
		w = modalMinWidth
	}
	h := height - chromeHeight
	if h < modalMinHeight {
		h = modalMinHeight
	}
	return w, h
}
