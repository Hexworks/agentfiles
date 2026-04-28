package asset

import (
	"errors"
	"testing"
)

func TestValidate_RejectsMissingIDOrName(t *testing.T) {
	cases := []Manifest{
		{},
		{Name: "name only", Type: TypeSkill},
		{ID: "id-only", Type: TypeSkill},
	}
	for _, m := range cases {
		err := m.Validate()
		if !errors.Is(err, ErrAssetIDNameRequired) {
			t.Fatalf("expected ErrAssetIDNameRequired, got %v", err)
		}
	}
}

func TestValidate_UnsupportedTypeReturnsTypedError(t *testing.T) {
	m := Manifest{ID: "x", Name: "X", Type: Type("totally-made-up")}

	err := m.Validate()

	var typed UnsupportedAssetTypeError
	if !errors.As(err, &typed) {
		t.Fatalf("expected UnsupportedAssetTypeError, got %T: %v", err, err)
	}
	if typed.Type != "totally-made-up" {
		t.Fatalf("expected type preserved, got %q", typed.Type)
	}
}

func TestValidate_AcceptsKnownTypes(t *testing.T) {
	for _, typ := range AllTypes() {
		m := Manifest{ID: "x", Name: "X", Type: typ}
		if err := m.Validate(); err != nil {
			t.Fatalf("type %q rejected unexpectedly: %v", typ, err)
		}
	}
}
