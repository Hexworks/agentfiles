package pathselector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafetyIsUnderRoot(t *testing.T) {
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
		{name: "empty root treats as unconstrained", path: sibling, root: "", want: true},
		{
			name: "sibling with common prefix does not qualify as ancestor",
			path: root + "-sibling",
			root: root,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isUnderRoot(tc.path, tc.root)
			if got != tc.want {
				t.Fatalf("isUnderRoot(%q, %q) = %v, want %v", tc.path, tc.root, got, tc.want)
			}
		})
	}
}

func TestSafetyIsUnderRootResolvesSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if isUnderRoot(link, root) {
		t.Fatal("symlink target outside root must not be reported as under root")
	}
}

func TestSafetyResolveAbsFollowExpandsSymlinks(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got := resolveAbs(link, true)
	wantTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(wantTarget) {
		t.Fatalf("resolveAbs(follow=true) = %q, want %q", got, wantTarget)
	}
}

func TestSafetyResolveAbsNoFollowLeavesSymlinks(t *testing.T) {
	target := t.TempDir()
	dir := t.TempDir()
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got := resolveAbs(link, false)
	if got != link {
		t.Fatalf("resolveAbs(follow=false) = %q, want %q (symlink preserved)", got, link)
	}
}
