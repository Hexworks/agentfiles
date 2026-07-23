package git

import "testing"

func TestCovers(t *testing.T) {
	cases := []struct {
		name     string
		pathspec []string
		path     string
		want     bool
	}{
		{"literal match", []string{"assets/foo/asset.json"}, "assets/foo/asset.json", true},
		{"literal miss", []string{"assets/foo/asset.json"}, "assets/bar/asset.json", false},
		{"wildcard descendant", []string{"assets/foo/**"}, "assets/foo/x/y/z.md", true},
		{"wildcard exact", []string{"assets/foo/**"}, "assets/foo", true},
		{"wildcard prefix miss", []string{"assets/foo/**"}, "assets/foobar/x.md", false},
		{"leading dot slash normalized", []string{"assets/foo/**"}, "./assets/foo/x.md", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Covers(tc.pathspec, tc.path); got != tc.want {
				t.Fatalf("Covers(%v, %q)=%v want %v", tc.pathspec, tc.path, got, tc.want)
			}
		})
	}
}
