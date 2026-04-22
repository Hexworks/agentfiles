package registry

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAddRejectsDuplicatePath(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	ref := ProfileRef{ID: "one", Name: "one", Path: "/tmp/one", CreatedAt: time.Now(), LastOpenedAt: time.Now()}
	if err := store.Add(ref); err != nil {
		t.Fatalf("add ref: %v", err)
	}
	err := store.Add(ProfileRef{ID: "two", Name: "two", Path: "/tmp/one", CreatedAt: time.Now(), LastOpenedAt: time.Now()})
	if err == nil {
		t.Fatal("expected duplicate path error")
	}
}
