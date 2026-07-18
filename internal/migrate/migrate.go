// Package migrate runs one-shot user-config migrations at startup. It is
// intentionally minimal: a single Run entry point invoked from main
// before the TUI opens, plus the v1→v2 migration for splitting projects
// out of the profile aggregate (ADR 0017).
//
// Design notes:
//   - Detection is presence-based: if either v2 file already exists Run
//     skips. Presence of only projects.json (a partial-migration
//     artifact) is treated as "already migrated" so subsequent launches
//     do not retry against a live v2 layout.
//   - Write-then-swap: v2 files are written atomically first; only after
//     both are durable does Run touch the v1 originals. A crash before
//     both v2 writes finish leaves v1 intact.
//   - Errors after the v2 files are durable are logged and do not roll
//     back — the invariant we preserve is "user data reaches v2 shape",
//     not "v1 originals are always gone by return".
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/utils"
)

// v1RegistryFileName is the legacy registry filename that lived directly
// under $HOME. Kept private so no other package accidentally reintroduces
// the v1 location.
const v1RegistryFileName = ".agentprofiles.json"

// v1ProjectsDirName is the legacy per-profile subdirectory that held
// individual project manifests. Kept private for the same reason.
const v1ProjectsDirName = "projects"

// v1ProfileRef mirrors the on-disk shape of the legacy registry entry.
// It lives here so the v1 schema stays isolated inside internal/migrate;
// a future divergence in registry.ProfileRef cannot silently re-encode
// v1 records because the mapping to registry.ProfileRef is explicit at
// writeV2 time.
type v1ProfileRef struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Source       string    `json:"source"`
	ManagedBy    string    `json:"managed_by"`
	CreatedAt    time.Time `json:"created_at"`
	LastOpenedAt time.Time `json:"last_opened_at"`
}

// v1Registry mirrors the on-disk shape of the legacy registry file.
type v1Registry struct {
	Version  int            `json:"version"`
	Profiles []v1ProfileRef `json:"profiles"`
}

// Logger is the callback used by Run to surface non-fatal migration
// events (stale profile paths, best-effort cleanup failures). It is
// injectable so tests can capture output without touching stderr.
// The severity classifies the kind of event: stale-path skips are
// warnings the operator should notice, cleanup best-effort failures are
// informational.
type Logger func(severity errs.Severity, msg string)

// StderrLogger writes msg to os.Stderr with a "migrate: " prefix. The
// severity prefix (warning / info) makes downstream filters possible
// without parsing the message body. Intended default for callers that
// want operational visibility.
func StderrLogger(severity errs.Severity, msg string) {
	fmt.Fprintln(os.Stderr, "migrate: "+severityLabel(severity)+": "+msg)
}

// severityLabel translates errs.Severity into its stable log-prefix
// spelling. Keeping the mapping local means the log format does not
// couple to the numeric ordering of the enum.
func severityLabel(s errs.Severity) string {
	switch s {
	case errs.SeverityInfo:
		return "info"
	case errs.SeverityWarning:
		return "warning"
	case errs.SeverityError:
		return "error"
	}
	return "unknown"
}

// discardLogger drops all messages. Used when Run is called with nil.
func discardLogger(errs.Severity, string) {}

// Run performs any pending user-config migrations. It is safe to call on
// every startup: presence-based detection makes it a no-op once the v2
// layout is in place. Callers pass the same store instances the app will
// use afterwards so paths stay consistent.
//
// The log callback receives non-fatal messages (stale profile paths,
// best-effort cleanup failures) with an errs.Severity classifying each
// event. Pass StderrLogger for the standard operational behavior, or a
// captured sink for tests. A nil callback is treated as "discard".
func Run(profileStore *registry.Store, projectStore *projectstore.Store, log Logger) errs.DomainError {
	if log == nil {
		log = discardLogger
	}
	if alreadyMigrated(profileStore, projectStore) {
		return nil
	}
	v1Path, ok := v1RegistryPath()
	if !ok || !utils.Exists(v1Path) {
		return nil
	}
	refs, err := readV1Registry(v1Path)
	if err != nil {
		return err
	}
	projectsByProfile, harvestErr := harvestV1(refs, log)
	if harvestErr != nil {
		return harvestErr
	}
	if err := writeV2(profileStore, projectStore, refs, projectsByProfile); err != nil {
		return err
	}
	cleanupV1(v1Path, refs, log)
	return nil
}

// alreadyMigrated is the presence-based idempotency gate. Either v2
// file's presence blocks a rerun so a partial-write left by a prior
// crash does not trigger the destructive cleanup phase against v1 a
// second time.
func alreadyMigrated(profileStore *registry.Store, projectStore *projectstore.Store) bool {
	return utils.Exists(profileStore.Path) || utils.Exists(projectStore.Path)
}

// harvestV1 walks every ref's projects/ directory and returns the
// decoded manifests grouped by profile id. Stale refs (folder gone,
// unsafe path shape) are logged with a warning and skipped so a single
// bad entry does not fail the whole migration. A structural failure
// (unreadable projects/ directory, invalid manifest) surfaces as a
// domain error because silent data loss is worse than a loud failure.
func harvestV1(refs []v1ProfileRef, log Logger) (map[string][]*project.Manifest, errs.DomainError) {
	projectsByProfile := map[string][]*project.Manifest{}
	for _, ref := range refs {
		if reason, unsafe := isUnsafeRefPath(ref.Path); unsafe {
			log(errs.SeverityWarning, "stale profile path skipped: "+ref.Path+" ("+reason+")")
			continue
		}
		projectsDir := filepath.Join(ref.Path, v1ProjectsDirName)
		info, statErr := os.Lstat(projectsDir)
		if statErr != nil || !info.IsDir() {
			log(errs.SeverityWarning, "stale profile path skipped: "+ref.Path)
			continue
		}
		manifests, harvestErr := harvestProjectsDir(projectsDir)
		if harvestErr != nil {
			return nil, harvestErr
		}
		if len(manifests) > 0 {
			projectsByProfile[ref.ID] = manifests
		}
	}
	return projectsByProfile, nil
}

// writeV2 writes the v2 stores atomically, projects first then profiles.
// Both writes must succeed before Run touches v1. Using atomic writes
// means a crash mid-write leaves the target path unchanged instead of a
// truncated file that would trip alreadyMigrated on the next launch.
func writeV2(profileStore *registry.Store, projectStore *projectstore.Store, refs []v1ProfileRef, projectsByProfile map[string][]*project.Manifest) errs.DomainError {
	if err := projectStore.Save(&projectstore.State{Version: projectstore.Version, Projects: projectsByProfile}); err != nil {
		return err
	}
	reg := &registry.Registry{Version: registry.Version, Profiles: refsToRegistry(refs)}
	return profileStore.Save(reg)
}

// cleanupV1 removes the v1 registry file and every v1 projects/ dir.
// Failures are logged as informational events (v2 is already durable so
// they are recoverable at leisure) rather than propagated.
func cleanupV1(v1Path string, refs []v1ProfileRef, log Logger) {
	if rmErr := os.Remove(v1Path); rmErr != nil && !os.IsNotExist(rmErr) {
		log(errs.SeverityInfo, "could not remove v1 registry "+v1Path+": "+rmErr.Error())
	}
	for _, ref := range refs {
		if _, unsafe := isUnsafeRefPath(ref.Path); unsafe {
			continue
		}
		projectsDir := filepath.Join(ref.Path, v1ProjectsDirName)
		if rmErr := os.RemoveAll(projectsDir); rmErr != nil {
			log(errs.SeverityInfo, "could not remove v1 projects dir "+projectsDir+": "+rmErr.Error())
		}
	}
}

// refsToRegistry translates the harvested v1 profile refs into the v2
// registry shape one field at a time. Naming the mapping explicitly is
// what keeps the v1 schema isolated: adding a v2-only field to
// registry.ProfileRef cannot silently re-encode v1 records because it
// would fail to compile here.
func refsToRegistry(refs []v1ProfileRef) []registry.ProfileRef {
	out := make([]registry.ProfileRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, registry.ProfileRef{
			ID:           ref.ID,
			Name:         ref.Name,
			Path:         ref.Path,
			Source:       ref.Source,
			ManagedBy:    ref.ManagedBy,
			CreatedAt:    ref.CreatedAt,
			LastOpenedAt: ref.LastOpenedAt,
		})
	}
	return out
}

// isUnsafeRefPath rejects ref.Path values that the v1 registry cannot be
// trusted to have validated: empty, relative (which would resolve
// against the process CWD), non-absolute after Clean, or a symlink. The
// v1 file is user-editable so a malicious or accidental entry could
// otherwise redirect the harvest / cleanup phase at arbitrary
// directories.
func isUnsafeRefPath(p string) (string, bool) {
	if p == "" {
		return "empty path", true
	}
	if !filepath.IsAbs(p) {
		return "not absolute", true
	}
	if filepath.Clean(p) != p {
		return "not clean", true
	}
	info, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "folder missing", true
		}
		return "unstatable", true
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink", true
	}
	if !info.IsDir() {
		return "not a directory", true
	}
	return "", false
}

// v1RegistryPath returns the absolute path of the v1 registry file. It
// reports false when the current process has no home directory (the same
// condition registry.DefaultPath surfaces as HomeDirUnavailableError for
// the v2 path), so Run can skip cleanly rather than acting on an empty
// prefix.
func v1RegistryPath() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, v1RegistryFileName), true
}

// readV1Registry decodes the v1 registry file into memory. It goes
// directly through encoding/json into the private v1Registry shape so
// the schema stays isolated inside this package; utils.ReadJSON is not
// used because the v1 shape is a migrate-internal concern, not a
// domain-side type.
func readV1Registry(path string) ([]v1ProfileRef, errs.DomainError) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ReadV1RegistryError{Path: path, Err: err}
	}
	var reg v1Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, ReadV1RegistryError{Path: path, Err: err}
	}
	if reg.Profiles == nil {
		return []v1ProfileRef{}, nil
	}
	return reg.Profiles, nil
}

// harvestProjectsDir reads every *.json file directly under dir and
// returns the decoded manifests. Malformed manifests surface as
// DomainErrors so the migration fails loudly rather than silently
// dropping user data.
func harvestProjectsDir(dir string) ([]*project.Manifest, errs.DomainError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, HarvestProjectsError{Path: dir, Err: err}
	}
	var out []*project.Manifest
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		manifest := &project.Manifest{}
		manifestPath := filepath.Join(dir, entry.Name())
		if readErr := utils.ReadJSON(manifestPath, manifest); readErr != nil {
			return nil, readErr
		}
		if normErr := manifest.Normalize(); normErr != nil {
			return nil, normErr
		}
		if valErr := manifest.Validate(); valErr != nil {
			return nil, valErr
		}
		out = append(out, manifest)
	}
	return out, nil
}
