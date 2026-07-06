// Package migrate runs one-shot user-config migrations at startup. It is
// intentionally minimal: a single Run entry point invoked from main
// before the TUI opens, plus the v1→v2 migration for splitting projects
// out of the profile aggregate (ADR 0017).
//
// Design notes:
//   - Detection is presence-based, not version-based, so the migration
//     stays idempotent even if a partial run is interrupted before the
//     v1 originals are deleted: the next launch sees v2 already present
//     and skips.
//   - Write-then-swap: v2 files are written first; only after they are
//     durable does Run touch the v1 originals. A crash before the v2
//     writes finish leaves v1 intact.
//   - Errors after the v2 files are durable are logged and do not roll
//     back — the invariant we preserve is "user data reaches v2 shape",
//     not "v1 originals are always gone by return".
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

// Logger is the callback used by Run to surface non-fatal migration
// events (stale profile paths, best-effort cleanup failures). It is
// injectable so tests can capture output without touching stderr.
type Logger func(string)

// StderrLogger writes msg to os.Stderr with a "migrate: " prefix.
// Intended default for callers that want operational visibility.
func StderrLogger(msg string) {
	fmt.Fprintln(os.Stderr, "migrate: "+msg)
}

// discardLogger drops all messages. Used when Run is called with nil.
func discardLogger(string) {}

// Run performs any pending user-config migrations. It is safe to call on
// every startup: presence-based detection makes it a no-op once the v2
// layout is in place. Callers pass the same store instances the app will
// use afterwards so paths stay consistent.
//
// The log callback receives non-fatal messages (stale profile paths,
// best-effort cleanup failures). Pass StderrLogger for the standard
// operational behavior, or a captured sink for tests. A nil callback is
// treated as "discard".
func Run(profileStore *registry.Store, projectStore *projectstore.Store, log Logger) errs.DomainError {
	if log == nil {
		log = discardLogger
	}
	if utils.Exists(profileStore.Path) || utils.Exists(projectStore.Path) {
		return nil
	}
	v1Path, ok := v1RegistryPath()
	if !ok {
		return nil
	}
	if !utils.Exists(v1Path) {
		return nil
	}
	refs, err := readV1Registry(v1Path)
	if err != nil {
		return err
	}
	projectsByProfile := map[string][]*project.Manifest{}
	for _, ref := range refs {
		projectsDir := filepath.Join(ref.Path, v1ProjectsDirName)
		if !utils.Exists(projectsDir) {
			log("stale profile path skipped: " + ref.Path)
			continue
		}
		manifests, harvestErr := harvestProjectsDir(projectsDir)
		if harvestErr != nil {
			return harvestErr
		}
		if len(manifests) > 0 {
			projectsByProfile[ref.ID] = manifests
		}
	}
	if err := projectStore.Save(projectsByProfile); err != nil {
		return err
	}
	reg := &registry.Registry{Version: registry.Version, Profiles: refs}
	if err := profileStore.Save(reg); err != nil {
		return err
	}
	if rmErr := os.Remove(v1Path); rmErr != nil && !os.IsNotExist(rmErr) {
		log("could not remove v1 registry " + v1Path + ": " + rmErr.Error())
	}
	for _, ref := range refs {
		projectsDir := filepath.Join(ref.Path, v1ProjectsDirName)
		if rmErr := os.RemoveAll(projectsDir); rmErr != nil {
			log("could not remove v1 projects dir " + projectsDir + ": " + rmErr.Error())
		}
	}
	return nil
}

// v1RegistryPath returns the absolute path of the v1 registry file. It
// reports false when the current process has no home directory (the same
// condition registry.DefaultPath silently tolerates for the v2 path), so
// Run can skip cleanly rather than acting on an empty prefix.
func v1RegistryPath() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, v1RegistryFileName), true
}

// readV1Registry decodes the v1 registry file into memory. It goes
// directly through encoding/json so the v1 schema stays isolated inside
// this package (utils.ReadJSON returns typed errors that reference the
// path, which is fine for surfacing).
func readV1Registry(path string) ([]registry.ProfileRef, errs.DomainError) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ReadV1RegistryError{Path: path, Err: err}
	}
	var reg registry.Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, ReadV1RegistryError{Path: path, Err: err}
	}
	if reg.Profiles == nil {
		return []registry.ProfileRef{}, nil
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
