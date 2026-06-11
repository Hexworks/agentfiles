package shell

import (
	"strings"
	"testing"
)

func TestWelcomeStub_TitleAndBody(t *testing.T) {
	s := newWelcomeStub()
	if got := s.Title(); got != "Welcome" {
		t.Errorf("Title() = %q, want %q", got, "Welcome")
	}
	if body := s.Body(80, 10); !strings.Contains(body, "agentfiles") {
		t.Errorf("Body missing 'agentfiles':\n%s", body)
	}
}
