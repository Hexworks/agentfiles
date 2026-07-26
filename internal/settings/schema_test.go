package settings

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/errs"
)

func TestLoad_RejectsNewerVersion(t *testing.T) {
	// given a settings file written by a newer build
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := writeString(path, `{"version":99,"git":{"enabled":true}}`+"\n"); err != nil {
		t.Fatalf("prewrite: %v", err)
	}

	// when loaded
	_, err := NewStore(path).Load()

	// then it is rejected
	var newer errs.NewerSchemaVersionError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerSchemaVersionError, got %T: %v", err, err)
	}
}
