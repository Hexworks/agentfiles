package project

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestValidate_MissingFields(t *testing.T) {
	m := &Manifest{}
	err := m.Validate()
	if !errors.Is(err, ErrProjectFieldsRequired) {
		t.Fatalf("expected ErrProjectFieldsRequired, got %v", err)
	}
}

func TestValidate_NoEnabledAgents(t *testing.T) {
	m := &Manifest{ID: "id", Name: "n", Path: "/p"}
	err := m.Validate()
	if !errors.Is(err, ErrNoEnabledAgents) {
		t.Fatalf("expected ErrNoEnabledAgents, got %v", err)
	}
}

func TestValidate_Ok(t *testing.T) {
	m := &Manifest{
		ID:            "id",
		Name:          "n",
		Path:          "/p",
		EnabledAgents: []string{"codex"},
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestNormalize_MakesPathAbsoluteAndSortsSlices(t *testing.T) {
	rel := filepath.Join(t.TempDir(), "repo")
	m := &Manifest{
		ID:               "id",
		Name:             "n",
		Path:             rel,
		EnabledAgents:    []string{"codex", "claude"},
		SelectedAssetIDs: []string{"b", "a"},
	}
	if err := m.Normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !filepath.IsAbs(m.Path) {
		t.Fatalf("expected absolute path, got %q", m.Path)
	}
	if m.EnabledAgents[0] != "claude" || m.SelectedAssetIDs[0] != "a" {
		t.Fatalf("expected sorted slices, got %+v", m)
	}
}

func TestSelectAsset_AppendsWhenAbsent(t *testing.T) {
	m := &Manifest{}
	if !m.SelectAsset("skill-a") {
		t.Fatalf("expected true on first insert")
	}
	if m.SelectAsset("skill-a") {
		t.Fatalf("expected false on duplicate insert")
	}
	if len(m.SelectedAssetIDs) != 1 {
		t.Fatalf("expected 1 selected id, got %d", len(m.SelectedAssetIDs))
	}
}
