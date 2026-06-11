package modals

import (
	"testing"

	"charm.land/huh/v2"
)

func TestCreateFile_ResolvesWithTypedInput(t *testing.T) {
	form, state := buildCreateFile(CreateFileInput{Path: "README.md"})
	state.Path = "scripts/install.sh"
	submitForm(t, form)

	msg := runResolvedThroughModal(t, "create-file", form, func(*huh.Form) any {
		return *state
	})

	got, ok := msg.Value.(CreateFileInput)
	if !ok {
		t.Fatalf("Value type = %T, want CreateFileInput", msg.Value)
	}
	if got.Path != "scripts/install.sh" {
		t.Errorf("Path = %q", got.Path)
	}
}

func TestCreateFile_CancelResolvesEmpty(t *testing.T) {
	form, state := buildCreateFile(CreateFileInput{Path: "x"})
	abortForm(form)

	msg := runResolvedThroughModal(t, "create-file", form, func(*huh.Form) any {
		return *state
	})

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
