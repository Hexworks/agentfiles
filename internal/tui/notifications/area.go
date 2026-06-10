package notifications

import tea "charm.land/bubbletea/v2"

// NotificationArea is the one-line bar rendered under each screen's
// content (above the status bar). It is a thin wrapper around Toast so
// the shell mounts a single component per screen and per-screen
// padding/width concerns have a single home.
type NotificationArea struct {
	toast *Toast
}

// NewArea wraps the given Toast widget. The Toast pointer is shared,
// not copied — the shell is responsible for owning a single Toast and
// reusing it across screens if desired.
func NewArea(toast *Toast) *NotificationArea {
	if toast == nil {
		panic("notifications.NewArea: nil toast")
	}
	return &NotificationArea{toast: toast}
}

// Init delegates to the wrapped Toast.
func (a *NotificationArea) Init() tea.Cmd { return a.toast.Init() }

// Update forwards every message to the wrapped Toast.
func (a *NotificationArea) Update(msg tea.Msg) (*NotificationArea, tea.Cmd) {
	var cmd tea.Cmd
	a.toast, cmd = a.toast.Update(msg)
	return a, cmd
}

// View renders the front of the toast queue, or empty string if empty.
func (a *NotificationArea) View() string { return a.toast.View() }

// Empty reports whether there is no visible toast.
func (a *NotificationArea) Empty() bool { return a.toast.Empty() }
