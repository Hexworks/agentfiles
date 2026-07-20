package pathselector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConstructorValidatesInputs(t *testing.T) {
	t.Parallel()

	t.Run("StartFolder outside constraint returns StartOutsideConstraintError", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		outside := t.TempDir()
		_, err := New(Options{ConstraintRoot: root, StartFolder: outside})
		if err == nil {
			t.Fatalf("expected StartOutsideConstraintError, got nil")
		}
		var typed StartOutsideConstraintError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StartOutsideConstraintError, got %T (%v)", err, err)
		}
	})

	t.Run("Unreadable StartFolder returns StartUnreadableError", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		missing := filepath.Join(root, "does-not-exist")
		_, err := New(Options{ConstraintRoot: root, StartFolder: missing})
		if err == nil {
			t.Fatalf("expected StartUnreadableError, got nil")
		}
		var typed StartUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StartUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("Unreadable ConstraintRoot returns ConstraintUnreadableError", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		_, err := New(Options{ConstraintRoot: missing})
		if err == nil {
			t.Fatalf("expected ConstraintUnreadableError, got nil")
		}
		var typed ConstraintUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected ConstraintUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("StartFolder is a regular file returns StartUnreadableError", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		file := filepath.Join(root, "not-a-dir")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := New(Options{ConstraintRoot: root, StartFolder: file})
		if err == nil {
			t.Fatalf("expected StartUnreadableError, got nil")
		}
		var typed StartUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StartUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("ConstraintRoot is a regular file returns ConstraintUnreadableError", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := New(Options{ConstraintRoot: file})
		if err == nil {
			t.Fatalf("expected ConstraintUnreadableError, got nil")
		}
		var typed ConstraintUnreadableError
		if !errors.As(err, &typed) {
			t.Fatalf("expected ConstraintUnreadableError, got %T (%v)", err, err)
		}
	})

	t.Run("Empty StartFolder falls back to ConstraintRoot", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		c, err := New(Options{ConstraintRoot: root})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.current != filepath.Clean(root) {
			t.Fatalf("current = %q, want %q", c.current, root)
		}
	})
}

func TestCanonicalizePureAndDeterministic(t *testing.T) {
	t.Parallel()
	opts := Options{
		Caption:           "  Pick file  ",
		ConstraintRoot:    "  /tmp  ",
		StartFolder:       " /tmp/start ",
		AllowedExtensions: []string{"md", ".JSON", " "},
		FollowSymlinks:    true,
	}
	canon := opts.canonicalize()
	if canon.caption != "Pick file" {
		t.Fatalf("caption = %q, want %q", canon.caption, "Pick file")
	}
	if canon.constraint != "/tmp" {
		t.Fatalf("constraint = %q, want %q", canon.constraint, "/tmp")
	}
	if canon.startInput != "/tmp/start" {
		t.Fatalf("startInput = %q, want %q", canon.startInput, "/tmp/start")
	}
	if _, ok := canon.allowedExt[".md"]; !ok {
		t.Fatalf("allowedExt missing .md: %v", canon.allowedExt)
	}
	if _, ok := canon.allowedExt[".json"]; !ok {
		t.Fatalf("allowedExt missing .json: %v", canon.allowedExt)
	}
	if !canon.followSymlinks {
		t.Fatalf("followSymlinks lost through canonicalize")
	}
}

func TestNormalizeExtensions(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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

func TestUserFacingPath(t *testing.T) {
	t.Parallel()
	if got := userFacingPath("~/foo", "/home/x/foo"); got != "~/foo" {
		t.Fatalf("input non-empty must win: got %q", got)
	}
	if got := userFacingPath("", "/home/x/foo"); got != "/home/x/foo" {
		t.Fatalf("empty input must fall back: got %q", got)
	}
}
