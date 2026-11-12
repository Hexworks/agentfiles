package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// AdoptAssetPickerID is the modal id of the Adopt folder picker.
const AdoptAssetPickerID = "adopt-asset-pick"

// NewAdoptAssetPicker builds the first step of the Edit Profile Adopt flow:
// a single-select over the profile's unadopted folders (directories under
// assets/<type>/ with no asset.json). The resolved payload is the picked
// folder's Key (`<type>/<name>`); the screen then opens the Create Asset
// form seeded from that folder and the service re-resolves the key. folders
// must be non-empty — the screen short-circuits to a notification otherwise.
func NewAdoptAssetPicker(folders []profile.UnadoptedFolder) *modal.Modal {
	form, extract := buildAdoptAssetPicker(folders)
	return modal.NewForm(AdoptAssetPickerID, form, extract, modal.WithCaption("Adopt Asset Folder"))
}

func buildAdoptAssetPicker(folders []profile.UnadoptedFolder) (*huh.Form, func(*huh.Form) any) {
	var key string
	options := make([]huh.Option[string], 0, len(folders))
	for _, f := range folders {
		options = append(options, huh.NewOption(f.Key, f.Key))
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("folder").
				Title("Folder").
				Description("Folders under assets/ that have no asset.json yet. Pick one to onboard it as an asset.").
				Value(&key).
				Options(options...),
		),
	).WithTheme(styles.HuhTheme())
	return form, func(*huh.Form) any { return key }
}
