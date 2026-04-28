package tui

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain forces lipgloss to ASCII output so test assertions can match plain
// substrings without dealing with embedded ANSI escape sequences. The CI
// environment may or may not be a TTY; this makes test output stable
// regardless.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}
