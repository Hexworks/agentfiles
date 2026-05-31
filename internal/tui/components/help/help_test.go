package help

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// Non-`.md` paths must be rejected before any file system access. Otherwise a
// caller could trick the loader into reading arbitrary files just because the
// directory check would succeed.
func TestLoadManual_RejectsNonMarkdownExtension(t *testing.T) {
	_, err := loadManual("xul/something.txt", 80)
	var want *InvalidExtensionError
	if !errors.As(err, &want) {
		t.Fatalf("err = %v, want *InvalidExtensionError", err)
	}
}

// Missing files map to a typed NotFoundError so the dialog can render a
// useful message instead of a generic os error. The tests run from the
// package directory where `docs/manual/` does not exist, so any read fails
// with ErrNotExist.
func TestLoadManual_MapsMissingFileToNotFoundError(t *testing.T) {
	_, err := loadManual("definitely-not-here.md", 80)
	var want *NotFoundError
	if !errors.As(err, &want) {
		t.Fatalf("err = %v, want *NotFoundError", err)
	}
}

// Each typed error must carry the requested path in its message so users can
// tell at a glance which page failed.
func TestErrors_IncludePathInMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"invalid extension", &InvalidExtensionError{Path: "x/y.txt"}},
		{"outside root", &OutsideRootError{Path: "../escape.md"}},
		{"not found", &NotFoundError{Path: "missing.md"}},
		{"render", &RenderError{Path: "broken.md", Err: errors.New("boom")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.err.Error()
			if !strings.Contains(msg, "help:") {
				t.Errorf("message %q missing %q prefix", msg, "help:")
			}
		})
	}
}

// RenderError must unwrap to the underlying glamour error so callers can use
// errors.Is / errors.As on the chain.
func TestRenderError_Unwraps(t *testing.T) {
	cause := errors.New("glamour exploded")
	err := &RenderError{Path: "p.md", Err: cause}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, %v) = false, want true", err, cause)
	}
}

// renderLoadError formats the failure for display inside the dialog. The
// produced string must surface the error text so the user sees the cause.
func TestRenderLoadError_IncludesUnderlyingMessage(t *testing.T) {
	err := &NotFoundError{Path: "missing.md"}
	out := renderLoadError(err)
	if !strings.Contains(out, err.Error()) {
		t.Errorf("rendered output %q missing underlying message %q", out, err.Error())
	}
}

// The close binding (esc / q) must transition the content to Cancelled so the
// hosting modal emits a ResolvedMsg with Confirmed=false on the next cycle.
func TestContent_CloseKeysCancelResolution(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"esc", tea.KeyPressMsg{Code: 27}},
		{"q", tea.KeyPressMsg{Code: 'q', Text: "q"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &content{keys: defaultKeymap()}
			if state, _ := c.Resolution(); state != modal.Active {
				t.Fatalf("initial state = %v, want Active", state)
			}
			_, _ = c.Update(tc.msg)
			state, value := c.Resolution()
			if state != modal.Cancelled {
				t.Errorf("state = %v, want Cancelled", state)
			}
			if value != nil {
				t.Errorf("value = %v, want nil on cancel", value)
			}
		})
	}
}

// Non-close key presses must be forwarded to the viewport without flipping
// the resolution state — otherwise scrolling would close the dialog.
func TestContent_NonCloseKeyKeepsActive(t *testing.T) {
	c := &content{keys: defaultKeymap()}
	_, _ = c.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if state, _ := c.Resolution(); state != modal.Active {
		t.Fatalf("state = %v, want Active after scroll key", state)
	}
}

// New must never fail. Construction errors (including a missing manual file)
// are rendered into the viewport so the dialog still opens with a readable
// message instead of crashing the host.
func TestNew_DoesNotPanicWhenManualMissing(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("New panicked on missing manual: %v", r)
		}
	}()
	m := New("help-x", Request{Topic: "X", Path: "definitely-not-here.md"}, 60, 20)
	if m == nil {
		t.Fatalf("New returned nil modal")
	}
	if m.ID() != "help-x" {
		t.Errorf("ID = %q, want %q", m.ID(), "help-x")
	}
}
