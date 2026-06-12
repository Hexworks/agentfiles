package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// selectProjectAssetsStub is the placeholder pushed by the Edit Profile
// screen's Projects-table `[select Assets]` action until task 0028 lands
// the real Select Project Assets screen.
type selectProjectAssetsStub struct {
	backOnlyScreenBase
	profileID string
	projectID string
}

func newSelectProjectAssetsStub(profileID, projectID string) *selectProjectAssetsStub {
	if profileID == "" {
		panic("shell.newSelectProjectAssetsStub: empty profileID")
	}
	if projectID == "" {
		panic("shell.newSelectProjectAssetsStub: empty projectID")
	}
	return &selectProjectAssetsStub{
		backOnlyScreenBase: newBackOnlyBase(),
		profileID:          profileID,
		projectID:          projectID,
	}
}

func (s *selectProjectAssetsStub) ProfileID() string { return s.profileID }
func (s *selectProjectAssetsStub) ProjectID() string { return s.projectID }

func (s *selectProjectAssetsStub) Init() tea.Cmd { return nil }

func (s *selectProjectAssetsStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if cmd, _ := s.handleMsg(msg); cmd != nil {
		return s, cmd
	}
	return s, nil
}

func (s *selectProjectAssetsStub) Title() string             { return "Select Project Assets" }
func (s *selectProjectAssetsStub) StatusKeys() []key.Binding { return s.statusKeys() }
func (s *selectProjectAssetsStub) InputFocused() bool        { return false }
func (s *selectProjectAssetsStub) Body(width int) string {
	sentence := fmt.Sprintf(" Selecting assets for project %q in profile %q — task 0028", s.projectID, s.profileID)
	return s.renderBody(width, sentence)
}
