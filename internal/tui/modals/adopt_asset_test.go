package modals

import (
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/profile"
)

func TestAdoptAssetPicker_ResolvesFirstFolderKeyByDefault(t *testing.T) {
	folders := []profile.UnadoptedFolder{
		{Key: "skill/alpha", Type: asset.TypeSkill, Name: "alpha"},
		{Key: "skill/beta", Type: asset.TypeSkill, Name: "beta"},
	}
	form, extract := buildAdoptAssetPicker(folders)
	submitForm(t, form)

	msg := runResolvedThroughModal(t, AdoptAssetPickerID, form, extract)

	got, ok := msg.Value.(string)
	if !ok {
		t.Fatalf("Value type = %T, want string", msg.Value)
	}
	if got != "skill/alpha" {
		t.Errorf("key = %q, want skill/alpha", got)
	}
}
