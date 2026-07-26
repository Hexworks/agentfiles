// Package settings owns the user-editable settings written to
// ~/.agentfiles/settings.json. It sits alongside the profile registry
// and project store so every centralized user config lives in the same
// directory. Callers load the value once at startup, wire it into
// app.Service, and persist any updates via the Store.
package settings

import "github.com/hexworks/agentfiles/internal/errs"

// Version is the current settings.json schema version.
const Version = 1

// GitSettings groups every setting that toggles git integration.
// Kept as its own struct so future git-related toggles do not need a
// schema migration.
type GitSettings struct {
	Enabled bool `json:"enabled"`
	// RunHooks opts the automated commit path into the repository's
	// commit-time hooks (pre-commit, prepare-commit-msg, commit-msg).
	// Default is false so `Repo.Commit` passes `--no-verify` and hostile
	// hook scripts checked out on a branch cannot execute silently
	// (ADR 0019). Users who rely on hooks (e.g. GPG signing enforcement)
	// enable this explicitly.
	RunHooks bool `json:"run_hooks"`
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

// Migrate stamps the legacy sentinel to Version; see utils.Persisted.
func (s *Settings) Migrate() errs.DomainError {
	if s.Version == 0 {
		s.Version = Version
	}
	return nil
}

// SchemaVersion reports this value's version and the current one; see
// utils.Persisted.
func (s *Settings) SchemaVersion() (have, known int) { return s.Version, Version }

// Validate has no domain shape beyond the version envelope the boundary
// already guards, so it is a no-op; see utils.Persisted.
func (s *Settings) Validate() errs.DomainError {
	return nil
}
