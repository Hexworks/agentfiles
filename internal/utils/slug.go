package utils

import "strings"

// Slug creates a stable file/id friendly name from user-facing input. Lower
// case, spaces and underscores collapse to dashes, anything outside
// `[a-z0-9-]` is dropped. Empty results fall back to the caller-supplied
// fallback so the choice of generic id (e.g. "asset" vs "project" vs
// "profile") stays with the domain rather than living in this helper.
func Slug(v, fallback string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, " ", "-")
	v = strings.ReplaceAll(v, "_", "-")
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}
