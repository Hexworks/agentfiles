package utils

import (
	"errors"
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

func TestCopyDirNonexistentSourceReturnsTypedError(t *testing.T) {
	src := filepath.Join(t.TempDir(), "missing")
	dst := filepath.Join(t.TempDir(), "out")

	err := CopyDir(src, dst)
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
	var typed CopyDirError
	if !errors.As(err, &typed) {
		t.Fatalf("expected CopyDirError, got %T: %v", err, err)
	}
}

func TestCopyDirEmptySourceCreatesNothing(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir empty source: %v", err)
	}
	if entries, _ := os.ReadDir(dst); len(entries) != 0 {
		t.Fatalf("expected empty destination, got %d entries", len(entries))
	}
}

func TestCopyDirNormalizesFileMode(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	p := filepath.Join(src, "exec.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o777); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	info, statErr := os.Stat(filepath.Join(dst, "exec.sh"))
	if statErr != nil {
		t.Fatalf("stat copied file: %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("copied mode = %o, want 0644", got)
	}
}

func TestDirStatsCountsRegularFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.txt"), "abc")
	writeTestFile(t, filepath.Join(root, "sub", "b.txt"), "de")

	count, size, err := DirStats(root)
	if err != nil {
		t.Fatalf("DirStats: %v", err)
	}
	if count != 2 || size != 5 {
		t.Fatalf("DirStats = (%d, %d), want (2, 5)", count, size)
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
