package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// editProfileStub is the placeholder pushed by the Profiles screen's row-
// level `[Edit]` action until task 0026 lands the real Edit Profile
// screen. It carries the selected profile id so the future implementation
// only needs to swap the body, not the call site.
type editProfileStub struct {
	profileID string
	back      *mnemonic.Button
}

func newEditProfileStub(profileID string) *editProfileStub {
	return &editProfileStub{
		profileID: profileID,
		back: mnemonic.New(
			"Back",
			'b',
			func() tea.Cmd { return popCmd() },
			mnemonic.WithExtraBindingKeys("esc"),
		),
	}
}

func (s *editProfileStub) Init() tea.Cmd { return nil }

func (s *editProfileStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	if s.back.Matches(kp) {
		return s, s.back.Trigger()
	}
	return s, nil
}

func (s *editProfileStub) Title() string { return "Edit Profile" }

// StatusKeys exposes [Back] for the same reason the Settings stub does:
// the body has no other visible cue for it.
func (s *editProfileStub) StatusKeys() []key.Binding {
	return []key.Binding{s.back.Binding()}
}

func (s *editProfileStub) Body(width, _ int) string {
	msg := fmt.Sprintf(" Editing profile %q — task 0026", s.profileID)
	back := lipgloss.PlaceHorizontal(width, lipgloss.Right, s.back.View())
	return lipgloss.JoinVertical(lipgloss.Left, msg, "", back)
}
