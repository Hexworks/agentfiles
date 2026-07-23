// Package settings owns the user-editable settings written to
// ~/.agentfiles/settings.json. It sits alongside the profile registry
// and project store so every centralized user config lives in the same
// directory. Callers load the value once at startup, wire it into
// app.Service, and persist any updates via the Store.
package settings

// Version is the current settings.json schema version.
const Version = 1

// GitSettings groups every setting that toggles git integration.
// Kept as its own struct so future git-related toggles do not need a
// schema migration.
type GitSettings struct {
	Enabled bool `json:"enabled"`
}

// Settings is the value persisted to settings.json. Zero-value is the
// disabled default so a missing file loads cleanly.
type Settings struct {
	Version int         `json:"version"`
	Git     GitSettings `json:"git"`
}

// Default returns the zero-value Settings the loader falls back to when
// the file is missing. Written as an explicit constructor so future
// defaults do not spread across load sites.
func Default() Settings {
	return Settings{Version: Version}
}
