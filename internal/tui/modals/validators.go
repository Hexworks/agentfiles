package modals

import (
	"errors"
	"strings"
)

// errRequired is the sentinel returned by required-field validators. The TUI
// surfaces it through `huh`'s built-in error rendering; callers don't need to
// inspect the value.
var errRequired = errors.New("required")

func requiredString(s string) error {
	if strings.TrimSpace(s) == "" {
		return errRequired
	}
	return nil
}

func requiredAgents(values []string) error {
	if len(values) == 0 {
		return errRequired
	}
	return nil
}

// parseTags turns a comma-separated string into a normalised tag list:
// trims whitespace around each entry and drops empties. Returns nil for an
// all-empty input so the resulting `asset.Manifest.Tags` stays omitempty.
func parseTags(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
