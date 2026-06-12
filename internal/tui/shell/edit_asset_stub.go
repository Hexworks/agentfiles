package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// editAssetStub is the placeholder pushed by the Edit Profile screen's
// Assets-table `[Edit]` action until task 0027 lands the real Edit Asset
// screen. Embeds backOnlyScreenBase so the [Back] wiring + WindowSizeMsg
// tracking + status-bar entry live in one place.
type editAssetStub struct {
	backOnlyScreenBase
	profileID string
	assetID   string
}

func newEditAssetStub(profileID, assetID string) *editAssetStub {
	if profileID == "" {
		panic("shell.newEditAssetStub: empty profileID")
	}
	if assetID == "" {
		panic("shell.newEditAssetStub: empty assetID")
	}
	return &editAssetStub{
		backOnlyScreenBase: newBackOnlyBase(),
		profileID:          profileID,
		assetID:            assetID,
	}
}

// ProfileID + AssetID expose the captured ids so tests don't reach into
// unexported fields; the real screen task 0027 ships will preserve the
// same accessors.
func (s *editAssetStub) ProfileID() string { return s.profileID }
func (s *editAssetStub) AssetID() string   { return s.assetID }

func (s *editAssetStub) Init() tea.Cmd { return nil }

func (s *editAssetStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if cmd, _ := s.handleMsg(msg); cmd != nil {
		return s, cmd
	}
	return s, nil
}

func (s *editAssetStub) Title() string             { return "Edit Asset" }
func (s *editAssetStub) StatusKeys() []key.Binding { return s.statusKeys() }
func (s *editAssetStub) Body(width int) string {
	sentence := fmt.Sprintf(" Editing asset %q in profile %q — task 0027", s.assetID, s.profileID)
	return s.renderBody(width, sentence)
}
