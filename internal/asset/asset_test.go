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

func TestInitFromFolder_CopiesContentAndWritesManifest(t *testing.T) {
	// given a source folder holding the asset's real content
	root := t.TempDir()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("real skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{ID: "something", Name: "something", Type: TypeSkill}

	// when an asset is created from that folder
	dir, err := InitFromFolder(root, manifest, source)
	if err != nil {
		t.Fatalf("InitFromFolder: %v", err)
	}

	// then the folder content is copied and the manifest is loadable
	if want := filepath.Join(root, "assets", "skill", "something"); dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	body, readErr := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if readErr != nil {
		t.Fatalf("read copied file: %v", readErr)
	}
	if string(body) != "real skill\n" {
		t.Fatalf("copied content = %q, want %q", string(body), "real skill\n")
	}
	loaded, loadErr := Load(dir)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if loaded.ID != "something" || loaded.Type != TypeSkill {
		t.Fatalf("manifest = %+v, want id=something type=skill", loaded.Manifest)
	}
}

func TestInitFromFolder_RejectsInvalidManifest(t *testing.T) {
	_, err := InitFromFolder(t.TempDir(), Manifest{Type: TypeSkill}, t.TempDir())
	if !errors.Is(err, ErrAssetIDNameRequired) {
		t.Fatalf("expected ErrAssetIDNameRequired, got %v", err)
	}
}

func TestInitFromFolder_SourceManifestDoesNotClobberAuthoritative(t *testing.T) {
	// given a source folder that itself contains a stray asset.json
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "asset.json"),
		[]byte(`{"id":"evil","name":"Evil","type":"rule"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// when the folder is registered as a skill asset
	dir, err := InitFromFolder(t.TempDir(), Manifest{ID: "good", Name: "Good", Type: TypeSkill}, source)
	if err != nil {
		t.Fatalf("InitFromFolder: %v", err)
	}

	// then the authoritative manifest survives the copy (written last)
	loaded, loadErr := Load(dir)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if loaded.ID != "good" || loaded.Type != TypeSkill {
		t.Fatalf("source asset.json clobbered ours: %+v", loaded.Manifest)
	}
}

func TestInitFromFolder_CopiesNestedSubtree(t *testing.T) {
	source := t.TempDir()
	writeSource(t, source, "SKILL.md", "top\n")
	writeSource(t, source, filepath.Join("sub", "inner.md"), "deep\n")

	dir, err := InitFromFolder(t.TempDir(), Manifest{ID: "deep", Name: "Deep", Type: TypeSkill}, source)
	if err != nil {
		t.Fatalf("InitFromFolder: %v", err)
	}
	body, readErr := os.ReadFile(filepath.Join(dir, "sub", "inner.md"))
	if readErr != nil {
		t.Fatalf("read nested copied file: %v", readErr)
	}
	if string(body) != "deep\n" {
		t.Fatalf("nested content = %q, want %q", string(body), "deep\n")
	}
}

func TestInitFromFolder_RejectsSkillWithoutStarterFile(t *testing.T) {
	source := t.TempDir()
	writeSource(t, source, "notes.txt", "no skill md\n")

	_, err := InitFromFolder(t.TempDir(), Manifest{ID: "x", Name: "X", Type: TypeSkill}, source)
	var typed MissingContentFileError
	if !errors.As(err, &typed) {
		t.Fatalf("expected MissingContentFileError, got %T: %v", err, err)
	}
}

func TestInitFromFolder_RejectsGenericTypeWithoutProjections(t *testing.T) {
	source := t.TempDir()
	writeSource(t, source, "rule.md", "content\n")

	_, err := InitFromFolder(t.TempDir(), Manifest{ID: "r", Name: "R", Type: TypeRule}, source)
	var typed MissingProjectionsError
	if !errors.As(err, &typed) {
		t.Fatalf("expected MissingProjectionsError, got %T: %v", err, err)
	}
}

func TestFolderRegisterableTypes_AreConventionTypesOnly(t *testing.T) {
	got := FolderRegisterableTypes()
	want := []Type{TypeSkill, TypeAgentsDoc, TypeSettings}
	if len(got) != len(want) {
		t.Fatalf("FolderRegisterableTypes = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("FolderRegisterableTypes = %v, want %v", got, want)
		}
	}
}

func writeSource(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
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
