package registry

import (
	"errors"
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

func TestRemove_DeletesProfileRef(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	ref := ProfileRef{ID: "alpha", Name: "Alpha", Path: "/tmp/alpha", CreatedAt: time.Now(), LastOpenedAt: time.Now()}
	if err := store.Add(ref); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := store.Remove("alpha"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	reg, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(reg.Profiles) != 0 {
		t.Fatalf("expected empty registry, got %d entries", len(reg.Profiles))
	}
}

func TestRemove_MissingIDReturnsProfileNotFoundError(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))

	err := store.Remove("does-not-exist")

	var typed ProfileNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProfileNotFoundError, got %T: %v", err, err)
	}
	if typed.Ref != "does-not-exist" {
		t.Fatalf("expected ref preserved, got %q", typed.Ref)
	}
}
