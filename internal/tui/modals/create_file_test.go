package modals

import (
	"testing"
)

func TestCreateFile_PrefillSeedsState(t *testing.T) {
	_, state, _ := buildCreateFile(CreateFileInput{Path: "README.md"})
	if state.Path != "README.md" {
		t.Errorf("Path = %q", state.Path)
	}
}

func TestCreateFile_PumpResolvesWithTypedInput(t *testing.T) {
	form, _, extract := buildCreateFile(CreateFileInput{Path: "scripts/install.sh"})
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-file", form, extract)

	got, ok := msg.Value.(CreateFileInput)
	if !ok {
		t.Fatalf("Value type = %T, want CreateFileInput", msg.Value)
	}
	if got.Path != "scripts/install.sh" {
		t.Errorf("Path = %q", got.Path)
	}
}

func TestCreateFile_RejectsEmptyRequiredField(t *testing.T) {
	form, _, _ := buildCreateFile(CreateFileInput{})
	expectFormStuck(t, form)
}

func TestCreateFile_CancelResolvesEmpty(t *testing.T) {
	form, _, extract := buildCreateFile(CreateFileInput{Path: "x"})
	abortForm(form)

	msg := runResolvedThroughModal(t, "create-file", form, extract)

	if msg.Confirmed || msg.Value != nil {
		t.Errorf("cancel resolved = %#v", msg)
	}
}

func TestNewCreateFile_UsesStableID(t *testing.T) {
	m := NewCreateFile(CreateFileInput{})
	if got := m.ID(); got != "create-file" {
		t.Errorf("ID = %q, want %q", got, "create-file")
	}
}
