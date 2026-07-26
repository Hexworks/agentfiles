package modals

import (
	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// Shared field constructors for the modal forms. Centralising the labels and
// validators here keeps Title casing, key strings, and required-ness in lock
// step across the six form modals.

func nameInput(value *string, description string) *huh.Input {
	return huh.NewInput().
		Key("name").
		Title("Name").
		Description(description).
		Value(value).
		Validate(requiredString)
}

func idInput(value *string, description string) *huh.Input {
	return huh.NewInput().
		Key("id").
		Title("ID").
		Description(description).
		Value(value)
}

func pathInput(value *string, description string) *huh.Input {
	return huh.NewInput().
		Key("path").
		Title("Path").
		Description(description).
		Value(value).
		Validate(requiredString)
}

// pathDisplayNote renders the pre-picked path from the pathselector step as a
// non-editable Note in the follow-on form. On any key press Note.Update
// returns NextField, so Enter advances the group without letting the
// keystroke leak back to the shell. The path is placed in Title(...) so it
// bypasses the mini-markdown renderer that Description runs on (`*`, `_`,
// “ ` “, `\`) and stays visually intact for common paths like
// `~/repos/my_repo`.
//
// pickedPath is passed through styles.Safe so control / bidi / zero-width
// runes in a raw filesystem path cannot reach the frame; Safe preserves
// \t and \n (see its doc).
func pathDisplayNote(pickedPath string, description string) *huh.Note {
	return huh.NewNote().
		Title(styles.Safe(pickedPath)).
		Description(description)
}

func enabledAgentsSelect(value *[]string, description string) *huh.MultiSelect[string] {
	return huh.NewMultiSelect[string]().
		Key("enabled_agents").
		Title("Enabled Agents").
		Description(description).
		Value(value).
		Options(AgentOptions()...).
		Validate(requiredAgents)
}

func compatibleAgentsSelect(value *[]string, description string) *huh.MultiSelect[string] {
	return huh.NewMultiSelect[string]().
		Key("compatible_agents").
		Title("Compatible Agents").
		Description(description).
		Value(value).
		Options(AgentOptions()...)
}

func descriptionText(value *string, description string) *huh.Text {
	return huh.NewText().
		Key("description").
		Title("Description").
		Description(description).
		Value(value).
		Validate(requiredString)
}

func tagsInput(value *string, description string) *huh.Input {
	return huh.NewInput().
		Key("tags").
		Title("Tags").
		Description(description).
		Value(value)
}

func exclusiveGroupInput(value *string, description string) *huh.Input {
	return huh.NewInput().
		Key("exclusive_group").
		Title("Exclusive Group").
		Description(description).
		Value(value)
}

func assetTypeSelect(value *asset.Type, description string, types []asset.Type) *huh.Select[asset.Type] {
	return huh.NewSelect[asset.Type]().
		Key("type").
		Title("Type").
		Description(description).
		Value(value).
		Options(assetTypeOptions(types)...)
}

func assetTypeOptions(types []asset.Type) []huh.Option[asset.Type] {
	out := make([]huh.Option[asset.Type], 0, len(types))
	for _, t := range types {
		out = append(out, huh.NewOption(string(t), t))
	}
	return out
}
