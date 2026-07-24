package render

import (
	"fmt"
	"strings"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
)

// UnsupportedRenderingError reports a selected asset whose (Agent, Type)
// pair has no registered render strategy. Build accumulates it — never
// short-circuits — so a project that pairs one unsupported combination
// with otherwise valid selections still reports every problem at once.
// It replaces the fall-through silence of the old per-type switch: a
// missing pair is now an explicit, typed failure.
type UnsupportedRenderingError struct {
	Agent agent.Agent
	Type  asset.Type
}

func (e UnsupportedRenderingError) Error() string {
	return fmt.Sprintf("missing render strategy for %s %s", e.Agent, e.Type)
}

func (UnsupportedRenderingError) Severity() errs.Severity {
	return errs.SeverityError
}

// ExclusiveGroupConflictError reports two or more selected assets that share a
// non-empty exclusive_group. AssetIDs lists every asset id that participated
// in the conflict so the TUI can show all of them at once.
type ExclusiveGroupConflictError struct {
	Group    string
	AssetIDs []string
}

func (e ExclusiveGroupConflictError) Error() string {
	return fmt.Sprintf("multiple assets selected in exclusive group %s: %s", e.Group, strings.Join(e.AssetIDs, ", "))
}

func (ExclusiveGroupConflictError) Severity() errs.Severity {
	return errs.SeverityError
}

// AssetNotFoundError reports a project-selected asset id that does not exist
// in the profile. The same name is used by the app package for symmetry —
// both report the same situation, qualified by package.
type AssetNotFoundError struct {
	AssetID string
}

func (e AssetNotFoundError) Error() string {
	return fmt.Sprintf("selected asset not found: %s", e.AssetID)
}

func (AssetNotFoundError) Severity() errs.Severity {
	return errs.SeverityError
}

// AssetSourceMissingError reports a missing source file inside an asset
// directory (e.g. a skill manifest with no SKILL.md body). RelPath is
// relative to the asset directory so the absolute path under the user's
// profile root never reaches the user.
type AssetSourceMissingError struct {
	AssetID string
	RelPath string
}

func (e AssetSourceMissingError) Error() string {
	return fmt.Sprintf("%s: source file missing: %s", e.AssetID, e.RelPath)
}

func (AssetSourceMissingError) Severity() errs.Severity {
	return errs.SeverityError
}

// AssetReadError reports a non-missing read failure on an asset source
// file. RelPath is relative to the asset directory; Op identifies the
// underlying syscall (read, stat, walk). Err preserves the underlying
// cause for errors.Unwrap, but the rendered message hides the absolute
// path.
type AssetReadError struct {
	AssetID string
	RelPath string
	Op      string
	Err     error
}

func (e AssetReadError) Error() string {
	return fmt.Sprintf("%s: %s %s: %s", e.AssetID, e.Op, e.RelPath, redact(e.Err))
}

func (AssetReadError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e AssetReadError) Unwrap() error {
	return e.Err
}

// TargetOutsideSurfacesError reports a projection whose Target lies outside
// the managed-surface fence. The Target is preserved so the TUI can flag the
// exact path.
type TargetOutsideSurfacesError struct {
	AssetID string
	Target  string
}

func (e TargetOutsideSurfacesError) Error() string {
	return fmt.Sprintf("%s: target outside managed surfaces: %s", e.AssetID, e.Target)
}

func (TargetOutsideSurfacesError) Severity() errs.Severity {
	return errs.SeverityWarning
}

// redact strips the absolute path embedded in *os.PathError-style values
// down to the bare reason. The asset-relative path is reported by the
// outer error; including the full filesystem path under the profile root
// would leak local layout into user-visible output.
func redact(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// *os.PathError formats as "op path: reason"; keep only "reason".
	if idx := strings.LastIndex(msg, ": "); idx >= 0 {
		return msg[idx+2:]
	}
	return msg
}
