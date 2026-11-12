package asset

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/config"
)

func seedUnadopted(t *testing.T, root string, rel string, files ...string) string {
	t.Helper()
	dir := filepath.Join(root, config.AssetsDirName, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAdoptFolder_InPlaceWritesManifestOnly(t *testing.T) {
	root := t.TempDir()
	src := seedUnadopted(t, root, "skill/copied", "SKILL.md", "extra.md")

	got, err := AdoptFolder(root, Manifest{ID: "copied", Name: "Copied", Type: TypeSkill}, src)
	if err != nil {
		t.Fatalf("AdoptFolder: %v", err)
	}
	if got != src {
		t.Errorf("dir = %q, want in-place %q", got, src)
	}
	for _, f := range []string{"SKILL.md", "extra.md", config.AssetManifestFileName} {
		if _, statErr := os.Stat(filepath.Join(src, f)); statErr != nil {
			t.Errorf("missing %s after adopt: %v", f, statErr)
		}
	}
	loaded, loadErr := Load(src)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if loaded.ID != "copied" || loaded.Type != TypeSkill {
		t.Errorf("loaded manifest = %+v", loaded.Manifest)
	}
}

func TestAdoptFolder_MovesWhenIDOrTypeDiffers(t *testing.T) {
	root := t.TempDir()
	src := seedUnadopted(t, root, "skill/Copied Skill", "SKILL.md")

	got, err := AdoptFolder(root, Manifest{ID: "copied-skill", Name: "Copied Skill", Type: TypeSkill}, src)
	if err != nil {
		t.Fatalf("AdoptFolder: %v", err)
	}
	want := filepath.Join(root, config.AssetsDirName, "skill", "copied-skill")
	if got != want {
		t.Errorf("dir = %q, want %q", got, want)
	}
	if _, statErr := os.Stat(src); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("source still present after move: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(want, "SKILL.md")); statErr != nil {
		t.Errorf("content not moved: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(want, config.AssetManifestFileName)); statErr != nil {
		t.Errorf("manifest not written: %v", statErr)
	}
}

func TestAdoptFolder_RefusesExistingTarget(t *testing.T) {
	root := t.TempDir()
	src := seedUnadopted(t, root, "skill/copied", "SKILL.md")
	seedUnadopted(t, root, "skill/taken", "SKILL.md")

	_, err := AdoptFolder(root, Manifest{ID: "taken", Name: "Taken", Type: TypeSkill}, src)
	var exists AssetDirExistsError
	if !errors.As(err, &exists) {
		t.Fatalf("err = %v, want AssetDirExistsError", err)
	}
	if _, statErr := os.Stat(filepath.Join(src, "SKILL.md")); statErr != nil {
		t.Errorf("source touched on refusal: %v", statErr)
	}
}

func TestAdoptFolder_RequiresContentFile(t *testing.T) {
	root := t.TempDir()
	src := seedUnadopted(t, root, "skill/empty", "notes.md")

	_, err := AdoptFolder(root, Manifest{ID: "empty", Name: "Empty", Type: TypeSkill}, src)
	var missing MissingContentFileError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingContentFileError", err)
	}
	if _, statErr := os.Stat(filepath.Join(src, config.AssetManifestFileName)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("manifest written despite rejection: %v", statErr)
	}
}
