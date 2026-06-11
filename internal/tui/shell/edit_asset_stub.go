package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// editAssetStub is the placeholder pushed by the Edit Profile screen's
// Assets-table `[Edit]` action until task 0027 lands the real Edit Asset
// screen. It carries the selected profile + asset ids so the future
// implementation only needs to swap the body, not the call site.
type editAssetStub struct {
	profileID string
	assetID   string
	back      *mnemonic.Button
}

func newEditAssetStub(profileID, assetID string) *editAssetStub {
	return &editAssetStub{
		profileID: profileID,
		assetID:   assetID,
		back: mnemonic.New(
			"Back",
			'b',
			func() tea.Cmd { return popCmd() },
			mnemonic.WithExtraBindingKeys("esc"),
		),
	}
}

func (s *editAssetStub) Init() tea.Cmd { return nil }

func (s *editAssetStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	if s.back.Matches(kp) {
		return s, s.back.Trigger()
	}
	return s, nil
}

func (s *editAssetStub) Title() string { return "Edit Asset" }

// StatusKeys exposes [Back] for the same reason the Settings stub does:
// the body has no other visible cue for it.
func (s *editAssetStub) StatusKeys() []key.Binding {
	return []key.Binding{s.back.Binding()}
}

func (s *editAssetStub) Body(width, _ int) string {
	msg := fmt.Sprintf(" Editing asset %q in profile %q — task 0027", s.assetID, s.profileID)
	back := lipgloss.PlaceHorizontal(width, lipgloss.Right, s.back.View())
	return lipgloss.JoinVertical(lipgloss.Left, msg, "", back)
}
