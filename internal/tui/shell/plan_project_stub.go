package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// planProjectStub is the placeholder pushed by the Edit Profile screen's
// Projects-table `[Plan]` action until task 0029 lands the real Plan
// Project screen. It carries the selected profile + project ids so the
// future implementation only needs to swap the body, not the call site.
type planProjectStub struct {
	profileID string
	projectID string
	back      *mnemonic.Button
}

func newPlanProjectStub(profileID, projectID string) *planProjectStub {
	return &planProjectStub{
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

func (s *planProjectStub) Init() tea.Cmd { return nil }

func (s *planProjectStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	if s.back.Matches(kp) {
		return s, s.back.Trigger()
	}
	return s, nil
}

func (s *planProjectStub) Title() string { return "Plan Project" }

func (s *planProjectStub) StatusKeys() []key.Binding {
	return []key.Binding{s.back.Binding()}
}

func (s *planProjectStub) Body(width, _ int) string {
	msg := fmt.Sprintf(" Planning project %q in profile %q — task 0029", s.projectID, s.profileID)
	back := lipgloss.PlaceHorizontal(width, lipgloss.Right, s.back.View())
	return lipgloss.JoinVertical(lipgloss.Left, msg, "", back)
}
