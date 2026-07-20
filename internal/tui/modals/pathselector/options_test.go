package pathselector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConstructorValidatesInputs(t *testing.T) {
	t.Run("StartFolder outside constraint returns StartOutsideConstraintError", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		_, err := New(Options{Constraint: root, StartFolder: outside})
		if err == nil {
			t.Fatalf("expected StartOutsideConstraintError, got nil")
		}
		var typed StartOutsideConstraintError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StartOutsideConstraintError, got %T (%v)", err, err)
		}
	})

	t.Run("Unreadable StartFolder returns StartUnreadableError", func(t *testing.T) {
		root := t.TempDir()
		missing := filepath.Join(root, "does-not-exist")
		_, err := New(Options{Constraint: root, StartFolder: missing})
		if err == nil {
			t.Fatalf("expected StartUnreadableError, got nil")
		}
		var typed StartUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StartUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("Unreadable Constraint returns ConstraintUnreadableError", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		_, err := New(Options{Constraint: missing})
		if err == nil {
			t.Fatalf("expected ConstraintUnreadableError, got nil")
		}
		var typed ConstraintUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected ConstraintUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("StartFolder is a regular file returns StartUnreadableError", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "not-a-dir")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := New(Options{Constraint: root, StartFolder: file})
		if err == nil {
			t.Fatalf("expected StartUnreadableError, got nil")
		}
		var typed StartUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StartUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("Constraint is a regular file returns ConstraintUnreadableError", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := New(Options{Constraint: file})
		if err == nil {
			t.Fatalf("expected ConstraintUnreadableError, got nil")
		}
		var typed ConstraintUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected ConstraintUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("Empty StartFolder falls back to Constraint", func(t *testing.T) {
		root := t.TempDir()
		c, err := New(Options{Constraint: root})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.current != filepath.Clean(root) {
			t.Fatalf("current = %q, want %q", c.current, root)
		}
	})
}

func TestNormalizeExtensions(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want map[string]struct{}
	}{
		{name: "nil", in: nil, want: nil},
		{name: "empty slice", in: []string{}, want: nil},
		{name: "only empties", in: []string{"", "   "}, want: nil},
		{
			name: "mixed casing and leading dot",
			in:   []string{"md", ".JSON", ".MD"},
			want: map[string]struct{}{".md": {}, ".json": {}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeExtensions(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("size = %d, want %d (got=%v)", len(got), len(tc.want), got)
			}
			for k := range tc.want {
				if _, ok := got[k]; !ok {
					t.Fatalf("missing key %q in %v", k, got)
				}
			}
		})
	}
}
