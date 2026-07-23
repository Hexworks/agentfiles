package actions

import (
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/settings"
)

// LoadSettings returns the currently active settings. There is no
// error case — settings are loaded once at startup and live in memory.
func (a *Actions) LoadSettings(struct{}) (settings.Settings, errs.DomainError) {
	return a.svc.Settings(), nil
}

// UpdateSettings persists next and swaps it in. Enabling git triggers
// a git-binary pre-flight; failure surfaces git.BinaryMissingError and
// leaves settings.json untouched.
func (a *Actions) UpdateSettings(next settings.Settings) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.UpdateSettings(next)
}
