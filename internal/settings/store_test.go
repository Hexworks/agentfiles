package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func writeString(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "settings.json"))

	// Missing file → defaults.
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() missing file: %v", err)
	}
	if got != Default() {
		t.Fatalf("expected default settings, got %+v", got)
	}

	// Write → readback matches.
	want := Settings{Version: Version, Git: GitSettings{Enabled: true}}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load() after Save: %v", err)
	}
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestLoadZeroVersionFills(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	// Simulate a hand-written file lacking the version key.
	if err := writeString(path, `{"git":{"enabled":true}}`+"\n"); err != nil {
		t.Fatalf("prewrite: %v", err)
	}
	store := NewStore(path)
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if got.Version != Version {
		t.Fatalf("expected version %d, got %d", Version, got.Version)
	}
	if !got.Git.Enabled {
		t.Fatalf("expected Git.Enabled=true, got false")
	}
}
