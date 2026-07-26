// Package profile represents a profile folder on disk: its manifest and
// the assets it contains. Per-user project selections live in a separate
// aggregate (see ADR 0017 + internal/projectstore); the app layer
// composes them at load time via app.LoadedProfile so this package stays
// free of any projects-store dependency.
package profile

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
)

// Version is the current profile manifest schema version written to
// profile.json by Init.
const Version = 1

// Manifest is the top-level metadata stored in profile.json.
type Manifest struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Migrate stamps a legacy (version 0) profile manifest up to the current
// schema version at the persistence boundary. Pointer receiver so the stamp
// lands on the decoded value.
func (m *Manifest) Migrate() errs.DomainError {
	if m.Version == 0 {
		m.Version = Version
	}
	return nil
}

// Validate rejects a profile manifest written by a newer build than this
// one understands (forward-compat guard). Pointer receiver so *Manifest
// satisfies utils.Persisted alongside Migrate.
func (m *Manifest) Validate() errs.DomainError {
	if m.Version > Version {
		return errs.NewerSchemaVersionError{Have: m.Version, Known: Version}
	}
	return nil
}

// Profile is the in-memory representation of a profile after scanning its
// asset subdirectory. Project selections live in a separate aggregate
// (see ADR 0017); callers that need both view them through
// app.LoadedProfile rather than reaching for a field on Profile.
type Profile struct {
	Root     string
	Manifest Manifest
	Assets   map[string]*asset.Asset
}

// Init scaffolds a brand-new profile root with the expected folder layout.
// The created directories mirror the current set of first-class asset types.
// No `projects/` directory is scaffolded — per-user selections live in the
// central projects store.
func Init(root, name string) (*Manifest, errs.DomainError) {
	root, absErr := utils.ToAbsolute(root)
	if absErr != nil {
		return nil, absErr
	}
	manifest := &Manifest{
		Version:   Version,
		ID:        slug(name),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	types := asset.AllTypes()
	dirs := make([]string, 0, len(types))
	for _, t := range types {
		dirs = append(dirs, filepath.Join(root, config.AssetsDirName, string(t)))
	}
	for _, dir := range dirs {
		if err := utils.EnsureDir(dir); err != nil {
			return nil, err
		}
	}
	if err := utils.WriteJSON(filepath.Join(root, config.ProfileManifestFileName), *manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// Load reads profile.json and scans assets/ to build the in-memory profile
// model. The Projects map is allocated empty; callers that need projects
// call the app-layer composer that pulls them from the projects store.
func Load(root string) (*Profile, errs.DomainError) {
	root, absErr := utils.ToAbsolute(root)
	if absErr != nil {
		return nil, absErr
	}
	manifest, err := utils.ReadJSON[Manifest](filepath.Join(root, config.ProfileManifestFileName))
	if err != nil {
		return nil, err
	}
	profile := &Profile{
		Root:     root,
		Manifest: manifest,
		Assets:   map[string]*asset.Asset{},
	}
	if err := loadAssetsInto(profile); err != nil {
		return nil, err
	}
	return profile, nil
}

// loadAssetsInto walks the assets tree and loads every directory that contains an
// asset.json file. Each asset id must be unique within one profile.
func loadAssetsInto(loaded *Profile) errs.DomainError {
	assetsRoot := filepath.Join(loaded.Root, config.AssetsDirName)
	var domainErr errs.DomainError
	walkErr := filepath.WalkDir(assetsRoot, func(path string, dir os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !dir.IsDir() {
			return nil
		}
		if path == assetsRoot {
			return nil
		}
		manifestPath := filepath.Join(path, config.AssetManifestFileName)
		if !utils.Exists(manifestPath) {
			return nil
		}
		a, loadErr := asset.Load(path)
		if loadErr != nil {
			domainErr = loadErr
			return filepath.SkipAll
		}
		if _, exists := loaded.Assets[a.ID]; exists {
			domainErr = DuplicateAssetIDError{ID: a.ID}
			return filepath.SkipAll
		}
		loaded.Assets[a.ID] = a
		return filepath.SkipDir
	})
	if domainErr != nil {
		return domainErr
	}
	if walkErr != nil {
		return AssetsScanError{Root: assetsRoot, Err: walkErr}
	}
	return nil
}

// AssetList returns assets sorted by display name so TUI rendering stays
// stable across reloads without each screen re-implementing the ordering
// rule.
func (l *Profile) AssetList() []*asset.Asset {
	list := make([]*asset.Asset, 0, len(l.Assets))
	for _, a := range l.Assets {
		list = append(list, a)
	}
	asset.SortByName(list)
	return list
}

// PartitionAssets splits the profile's AssetList into selected and
// available slices using selectedIDs as the membership rule. Both halves
// keep the AssetList ordering so screens render the same vocabulary in
// the same order without re-implementing the partition or the sort.
// Asset ids in selectedIDs that are not present in the profile are
// silently dropped — render handles the "missing asset" error path.
func (l *Profile) PartitionAssets(selectedIDs []string) (selected, available []*asset.Asset) {
	inSelection := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		inSelection[id] = struct{}{}
	}
	for _, a := range l.AssetList() {
		if _, ok := inSelection[a.ID]; ok {
			selected = append(selected, a)
		} else {
			available = append(available, a)
		}
	}
	return selected, available
}

// slug converts a profile name into a stable id suitable for manifest storage.
func slug(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	var b strings.Builder
	lastDash := false
	for _, r := range v {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return config.DefaultProfileSlug
	}
	return out
}
