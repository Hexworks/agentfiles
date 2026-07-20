package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsAncestor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ancestor   string
		descendant string
		want       bool
	}{
		{name: "strict ancestor", ancestor: "/a", descendant: "/a/b", want: true},
		{name: "same path is not ancestor", ancestor: "/a", descendant: "/a", want: false},
		{name: "sibling with common prefix", ancestor: "/a", descendant: "/a-sibling", want: false},
		{name: "unrelated path", ancestor: "/a", descendant: "/b", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsAncestor(tc.ancestor, tc.descendant); got != tc.want {
				t.Fatalf("IsAncestor(%q, %q) = %v, want %v", tc.ancestor, tc.descendant, got, tc.want)
			}
		})
	}
}

func TestIsUnderRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := t.TempDir()

	tests := []struct {
		name string
		path string
		root string
		want bool
	}{
		{name: "same path", path: root, root: root, want: true},
		{name: "child under root", path: inner, root: root, want: true},
		{name: "sibling outside root", path: sibling, root: root, want: false},
		{name: "empty root is unconstrained", path: sibling, root: "", want: true},
		{name: "sibling with common prefix", path: root + "-sibling", root: root, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUnderRoot(tc.path, tc.root); got != tc.want {
				t.Fatalf("IsUnderRoot(%q, %q) = %v, want %v", tc.path, tc.root, got, tc.want)
			}
		})
	}
}

func TestIsUnderRootResolvesSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if IsUnderRoot(link, root) {
		t.Fatal("symlink target outside root must not be reported as under root")
	}
}

func TestIsUnderRootFailsClosedOnUnresolvableSide(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// A path whose leaf does not exist cannot be EvalSymlinks-resolved;
	// fail-closed means IsUnderRoot returns false even though the lexical
	// path clearly sits under root.
	missing := filepath.Join(root, "does-not-exist")
	if IsUnderRoot(missing, root) {
		t.Fatal("unresolvable path must fail closed")
	}
}

func TestResolveAbsNoFollowKeepsSymlinks(t *testing.T) {
	t.Parallel()
	target := t.TempDir()
	dir := t.TempDir()
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAbs(link, false)
	if err != nil {
		t.Fatalf("ResolveAbs: %v", err)
	}
	if got != link {
		t.Fatalf("ResolveAbs(follow=false) = %q, want %q", got, link)
	}
}

func TestResolveAbsFollowExpandsSymlinks(t *testing.T) {
	t.Parallel()
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAbs(link, true)
	if err != nil {
		t.Fatalf("ResolveAbs: %v", err)
	}
	wantTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(wantTarget) {
		t.Fatalf("ResolveAbs(follow=true) = %q, want %q", got, wantTarget)
	}
}

func TestResolveAbsFollowFailsOnDanglingSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "gone"), link); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveAbs(link, true)
	if err == nil {
		t.Fatal("expected PathResolutionError for dangling symlink")
	}
	var typed PathResolutionError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T (%v), want PathResolutionError", err, err)
	}
}
