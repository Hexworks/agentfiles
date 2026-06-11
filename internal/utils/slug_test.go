package utils

import "testing"

func TestSlug(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"lowercases ascii", "Hello", "hello"},
		{"spaces become dashes", "Hello World", "hello-world"},
		{"underscores become dashes", "snake_case_name", "snake-case-name"},
		{"trims and collapses", "  Mixed Case  ", "mixed-case"},
		{"drops non-ascii letters", "café", "caf"},
		{"keeps digits and dashes", "abc-123", "abc-123"},
		{"empty falls back to item", "", "item"},
		{"only invalid falls back to item", "!!!", "item"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Slug(tc.in); got != tc.want {
				t.Fatalf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
