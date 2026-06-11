package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// selectProjectAssetsStub is the placeholder pushed by the Edit Profile
// screen's Projects-table `[select Assets]` action until task 0028 lands
// the real Select Project Assets screen. It carries the selected profile +
// project ids so the future implementation only needs to swap the body,
// not the call site.
type selectProjectAssetsStub struct {
	profileID string
	projectID string
	back      *mnemonic.Button
}

func newSelectProjectAssetsStub(profileID, projectID string) *selectProjectAssetsStub {
	return &selectProjectAssetsStub{
		profileID: profileID,
		projectID: projectID,
		back: mnemonic.New(
			"Back",
			'b',
			func() tea.Cmd { return popCmd() },
			mnemonic.WithExtraBindingKeys("esc"),
		),
	}
}

func (s *selectProjectAssetsStub) Init() tea.Cmd { return nil }

func (s *selectProjectAssetsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	if s.back.Matches(kp) {
		return s, s.back.Trigger()
	}
	return s, nil
}

func (s *selectProjectAssetsStub) Title() string { return "Select Project Assets" }

func (s *selectProjectAssetsStub) StatusKeys() []key.Binding {
	return []key.Binding{s.back.Binding()}
}

func (s *selectProjectAssetsStub) Body(width, _ int) string {
	msg := fmt.Sprintf(" Selecting assets for project %q in profile %q — task 0028", s.projectID, s.profileID)
	back := lipgloss.PlaceHorizontal(width, lipgloss.Right, s.back.View())
	return lipgloss.JoinVertical(lipgloss.Left, msg, "", back)
}
