package asset

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFile_ContainmentGate_RejectsEscapeAndReservedNames pins
// that Adopt cannot pierce the asset folder even with hand-crafted
// SourceRel values. Covers the negative branches that the happy-path
// TestService_Apply_Adopt* tests do not exercise: dotfile, "..",
// asset.json, and symlink-target escape. See task 0035 review issue
// #12.
func TestWriteFile_ContainmentGate_RejectsEscapeAndReservedNames(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		rel  string
	}{
		{"dotfile", ".env"},
		{"dotfile in subdir", "sub/.secret"},
		{"parent escape", "../evil"},
		{"parent escape via subdir", "sub/../../evil"},
		{"manifest reserved", "asset.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := WriteFile(dir, tc.rel, []byte("payload"), 0o644)
			if err == nil {
				t.Fatalf("expected FilePathError for %q, got nil", tc.rel)
			}
			var typed FilePathError
			if !errors.As(err, &typed) {
				t.Fatalf("expected FilePathError for %q, got %T: %v", tc.rel, err, err)
			}
		})
	}
}

// TestWriteFile_SymlinkTargetEscape_IsRejected pins that placing a
// symlink inside the asset folder pointing at an outside path cannot
// smuggle an Adopt write past the containment rail. resolveExisting
// EvalSymlinks-expands the parent, and ResolveRelative rejects the
// resulting path.
func TestWriteFile_SymlinkTargetEscape_IsRejected(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(dir, "sub")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := WriteFile(dir, "sub/evil.md", []byte("payload"), 0o644)
	if err == nil {
		t.Fatalf("expected FilePathError for symlink escape, got nil")
	}
	var typed FilePathError
	if !errors.As(err, &typed) {
		t.Fatalf("expected FilePathError, got %T: %v", err, err)
	}
	// Body must not appear inside the outside directory.
	if _, statErr := os.Stat(filepath.Join(outside, "evil.md")); !os.IsNotExist(statErr) {
		t.Fatalf("body written through symlink: stat err = %v", statErr)
	}
}

// TestWriteFile_HappyPath_WritesBodyWithMode pins the positive
// contract: containment-safe rels succeed and preserve the requested
// mode bits so the Adopt reverse-write does not silently normalize
// executable files to 0o644.
func TestWriteFile_HappyPath_WritesBodyWithMode(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFile(dir, "SKILL.md", []byte("body"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
	body, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "body" {
		t.Fatalf("body = %q, want %q", body, "body")
	}
}
