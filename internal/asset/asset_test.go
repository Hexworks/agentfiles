package asset

import (
	"errors"
	"os"
	"path/filepath"
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

func TestDelete_RemovesDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skill", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Delete(dir); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected dir removed, stat err = %v", err)
	}
}

func TestDelete_MissingDirectoryIsIdempotent(t *testing.T) {
	if err := Delete(filepath.Join(t.TempDir(), "never-existed")); err != nil {
		t.Fatalf("first delete: %v", err)
	}
}

func TestDelete_RealFailureReturnsTypedError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based failure injection cannot run as root")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "asset")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Drop write/exec on parent so RemoveAll fails partway.
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	err := Delete(dir)

	var typed AssetFolderRemoveError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetFolderRemoveError, got %T: %v", err, err)
	}
	if typed.Unwrap() == nil {
		t.Fatal("expected wrapped os error preserved")
	}
}
