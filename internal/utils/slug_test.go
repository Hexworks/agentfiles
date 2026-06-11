package utils

import "testing"

func TestSlug(t *testing.T) {
	cases := []struct {
		name, in, fallback, want string
	}{
		{"lowercases ascii", "Hello", "x", "hello"},
		{"spaces become dashes", "Hello World", "x", "hello-world"},
		{"underscores become dashes", "snake_case_name", "x", "snake-case-name"},
		{"trims and collapses", "  Mixed Case  ", "x", "mixed-case"},
		{"drops non-ascii letters", "café", "x", "caf"},
		{"keeps digits and dashes", "abc-123", "x", "abc-123"},
		{"empty falls back", "", "asset", "asset"},
		{"only invalid falls back", "!!!", "project", "project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Slug(tc.in, tc.fallback); got != tc.want {
				t.Fatalf("Slug(%q, %q) = %q, want %q", tc.in, tc.fallback, got, tc.want)
			}
		})
	}
}
