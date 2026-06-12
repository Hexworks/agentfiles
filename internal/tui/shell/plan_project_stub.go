package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// planProjectStub is the placeholder pushed by the Edit Profile screen's
// Projects-table `[Plan]` action until task 0029 lands the real Plan
// Project screen.
type planProjectStub struct {
	backOnlyScreenBase
	profileID string
	projectID string
}

func newPlanProjectStub(profileID, projectID string) *planProjectStub {
	if profileID == "" {
		panic("shell.newPlanProjectStub: empty profileID")
	}
	if projectID == "" {
		panic("shell.newPlanProjectStub: empty projectID")
	}
	return &planProjectStub{
		backOnlyScreenBase: newBackOnlyBase(),
		profileID:          profileID,
		projectID:          projectID,
	}
}

func (s *planProjectStub) ProfileID() string { return s.profileID }
func (s *planProjectStub) ProjectID() string { return s.projectID }

func (s *planProjectStub) Init() tea.Cmd { return nil }

func (s *planProjectStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if cmd, _ := s.handleMsg(msg); cmd != nil {
		return s, cmd
	}
	return s, nil
}

func (s *planProjectStub) Title() string             { return "Plan Project" }
func (s *planProjectStub) StatusKeys() []key.Binding { return s.statusKeys() }
func (s *planProjectStub) Body(width, height int) string {
	sentence := fmt.Sprintf(" Planning project %q in profile %q — task 0029", s.projectID, s.profileID)
	return s.renderBody(width, height, sentence)
}
