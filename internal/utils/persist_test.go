package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/errs"
)

// fakeDoc is a local Persisted type used to exercise the generic JSON
// persistence boundary without depending on any domain package. Validate
// fails on an empty Name so the write-refuses-invalid path can be driven.
type fakeDoc struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

const fakeDocVersion = 1

func (d *fakeDoc) Migrate() errs.DomainError {
	if d.Version == 0 {
		d.Version = fakeDocVersion
	}
	return nil
}

func (d *fakeDoc) Validate() errs.DomainError {
	if d.Version > fakeDocVersion {
		return errs.NewerSchemaVersionError{Have: d.Version, Known: fakeDocVersion}
	}
	if d.Name == "" {
		return fakeDocInvalidError{}
	}
	return nil
}

type fakeDocInvalidError struct{}

func (fakeDocInvalidError) Error() string           { return "fake doc invalid: name required" }
func (fakeDocInvalidError) Severity() errs.Severity { return errs.SeverityError }

func TestReadJSON_RoundTrip(t *testing.T) {
	// given a value written through the boundary (version 0 in memory)
	path := filepath.Join(t.TempDir(), "nested", "doc.json")
	if err := WriteJSON(path, fakeDoc{Name: "alpha"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// when it is read back
	got, err := ReadJSON[fakeDoc](path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// then the value round-trips with the schema version stamped current
	if got.Name != "alpha" {
		t.Fatalf("name = %q, want alpha", got.Name)
	}
	if got.Version != fakeDocVersion {
		t.Fatalf("version = %d, want %d", got.Version, fakeDocVersion)
	}
}

func TestReadJSON_LegacySentinel(t *testing.T) {
	// given a raw file on disk with no version key (pre-versioning legacy)
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte(`{"name":"legacy"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// when read
	got, err := ReadJSON[fakeDoc](path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// then Migrate stamped the version up to current
	if got.Version != fakeDocVersion {
		t.Fatalf("version = %d, want %d (legacy sentinel migrated)", got.Version, fakeDocVersion)
	}
}

func TestReadJSON_RejectsNewerVersion(t *testing.T) {
	// given a file written by a newer build than this one knows
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"name":"future"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// when read
	_, err := ReadJSON[fakeDoc](path)

	// then it is rejected with a path-enriched NewerSchemaVersionError
	var newer errs.NewerSchemaVersionError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerSchemaVersionError, got %T: %v", err, err)
	}
	if newer.Have != 99 || newer.Known != fakeDocVersion {
		t.Fatalf("have/known = %d/%d, want 99/%d", newer.Have, newer.Known, fakeDocVersion)
	}
	if newer.Path != path {
		t.Fatalf("path = %q, want %q (enriched at boundary)", newer.Path, path)
	}
}

func TestReadJSON_Malformed(t *testing.T) {
	// given a garbage file
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when read
	_, err := ReadJSON[fakeDoc](path)

	// then it surfaces as ReadJSONError
	var readErr ReadJSONError
	if !errors.As(err, &readErr) {
		t.Fatalf("expected ReadJSONError, got %T: %v", err, err)
	}
}

func TestWriteJSON_RefusesInvalid(t *testing.T) {
	// given a value that fails Validate (empty Name)
	path := filepath.Join(t.TempDir(), "sub", "doc.json")

	// when written
	err := WriteJSON(path, fakeDoc{Name: ""})

	// then the write is refused and nothing lands on disk
	var invalid fakeDocInvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected fakeDocInvalidError, got %T: %v", err, err)
	}
	if Exists(path) {
		t.Fatal("invalid value must not be persisted")
	}
}

func TestWriteJSONAtomic_RefusesInvalidAndLeavesNoResidue(t *testing.T) {
	// given a value that fails Validate, written atomically into an existing dir
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.json")

	// when written atomically
	err := WriteJSONAtomic(path, fakeDoc{Name: ""}, 0o700, 0o600)

	// then it is refused, the target is absent, and no temp file remains
	var invalid fakeDocInvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected fakeDocInvalidError, got %T: %v", err, err)
	}
	if Exists(path) {
		t.Fatal("invalid value must not be persisted")
	}
	residue, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(residue) != 0 {
		t.Fatalf("expected no .tmp-* residue, found %v", residue)
	}
}

func TestWriteJSONAtomic_RoundTrip(t *testing.T) {
	// given a valid value written atomically
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := WriteJSONAtomic(path, fakeDoc{Name: "beta"}, 0o700, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// when read back
	got, err := ReadJSON[fakeDoc](path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// then the value round-trips with version stamped
	if got.Name != "beta" || got.Version != fakeDocVersion {
		t.Fatalf("got %+v, want {Version:%d Name:beta}", got, fakeDocVersion)
	}
}
