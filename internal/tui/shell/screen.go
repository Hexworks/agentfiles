// Package shell implements the root Bubble Tea program for agentfiles.
// It runs in alt-screen mode, owns a screen-router stack, intercepts
// the global key set (n notifications, s settings, ? info, q quit),
// hosts the notifications Log + Toast widget mounted above the status
// bar, and delegates everything else to the active (top-of-stack)
// Screen.
//
// Screens implement the [Screen] interface. They push or pop other
// screens by emitting [PushScreenMsg] / [PopScreenMsg] from their
// Update; the root model is the only place the stack mutates. Entity
// screens (Profiles, Edit Profile, …) are added in later tasks
// (0024–0029); the package ships a placeholder Welcome stub plus
// stubs for the three global-key destinations so the shell is
// observable and testable on its own.
package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Screen is the unit of navigation on the shell's stack.
//
// Body receives the content-area dimensions the shell has reserved
// (everything inside the window minus the title bar, toast line, and
// status bar). The shell controls layout; the screen renders into the
// area it is given.
//
// StatusKeys returns the dynamic bindings the status bar appends to
// the global set — typically the focused row's mnemonic-button
// bindings. The screen's labelled buttons (e.g. `[Create]`) must not
// appear here: they are already visible on the screen body and must
// not be duplicated in the bar.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	Body(width, height int) string
	Title() string
	StatusKeys() []key.Binding
}

// PushScreenMsg pushes Screen onto the shell's stack. The shell runs
// the new screen's Init after the push.
type PushScreenMsg struct{ Screen Screen }

// PopScreenMsg removes the top screen. On a single-screen stack it is
// a no-op (the root screen cannot be popped).
type PopScreenMsg struct{}
