package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirCopiesTreePreservingStructure(t *testing.T) {
	// given a nested source tree
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "top\n")
	writeTestFile(t, filepath.Join(src, "nested", "inner.txt"), "deep\n")

	// when the tree is copied
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir returned error: %v", err)
	}

	// then every file lands at the mirrored relative path with its content
	assertFileContent(t, filepath.Join(dst, "SKILL.md"), "top\n")
	assertFileContent(t, filepath.Join(dst, "nested", "inner.txt"), "deep\n")
}

func TestCopyDirSkipsSymlinks(t *testing.T) {
	// given a source tree containing a symlink to an outside file
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "secret.txt"), "secret\n")
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "real.txt"), "real\n")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(src, "link.txt")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "out")

	// when the tree is copied
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir returned error: %v", err)
	}

	// then the regular file is copied and the symlink is not followed
	assertFileContent(t, filepath.Join(dst, "real.txt"), "real\n")
	if Exists(filepath.Join(dst, "link.txt")) {
		t.Fatalf("symlink was copied into destination")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("content of %s = %q, want %q", path, string(got), want)
	}
}
