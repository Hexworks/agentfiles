// Package profile represents a profile folder on disk: its manifest, the
// assets it contains, and the projects it owns. It is the authoritative source
// of truth that render and sync consume.
package profile

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/addamsson/agentfiles/internal/asset"
	"github.com/addamsson/agentfiles/internal/config"
	"github.com/addamsson/agentfiles/internal/errs"
	"github.com/addamsson/agentfiles/internal/fsutil"
	"github.com/addamsson/agentfiles/internal/project"
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

// Profile is the in-memory representation of a profile after scanning its asset
// and project subdirectories. This is the object most domain operations work on.
type Profile struct {
	Root     string
	Manifest Manifest
	Assets   map[string]*asset.Asset
	Projects map[string]*project.Project
}

// Init scaffolds a brand-new profile root with the expected folder layout.
// The created directories mirror the current set of first-class asset types.
func Init(root, name string) (*Manifest, errs.DomainError) {
	root, absErr := fsutil.ToAbsolute(root)
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
	dirs := make([]string, 0, len(types)+1)
	for _, t := range types {
		dirs = append(dirs, filepath.Join(root, config.AssetsDirName, string(t)))
	}
	dirs = append(dirs, filepath.Join(root, config.ProjectsDirName))
	for _, dir := range dirs {
		if err := fsutil.EnsureDir(dir); err != nil {
			return nil, err
		}
	}
	if err := fsutil.WriteJSON(filepath.Join(root, config.ProfileManifestFileName), manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// Load reads profile.json and then scans assets/ and projects/ to build the
// complete in-memory profile model.
func Load(root string) (*Profile, errs.DomainError) {
	root, absErr := fsutil.ToAbsolute(root)
	if absErr != nil {
		return nil, absErr
	}
	var manifest Manifest
	if err := fsutil.ReadJSON(filepath.Join(root, config.ProfileManifestFileName), &manifest); err != nil {
		return nil, err
	}
	loaded := &Profile{
		Root:     root,
		Manifest: manifest,
		Assets:   map[string]*asset.Asset{},
		Projects: map[string]*project.Project{},
	}
	if err := scanAssets(loaded); err != nil {
		return nil, err
	}
	if err := scanProjects(loaded); err != nil {
		return nil, err
	}
	return loaded, nil
}

// scanAssets walks the assets tree and loads every directory that contains an
// asset.json file. Each asset id must be unique within one profile.
func scanAssets(loaded *Profile) errs.DomainError {
	assetsRoot := filepath.Join(loaded.Root, config.AssetsDirName)
	var domainErr errs.DomainError
	walkErr := filepath.WalkDir(assetsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path == assetsRoot {
			return nil
		}
		manifestPath := filepath.Join(path, config.AssetManifestFileName)
		if !fsutil.Exists(manifestPath) {
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

// scanProjects loads all project manifests from the profile's projects/
// directory and normalizes them before exposing them to the rest of the app.
func scanProjects(loaded *Profile) errs.DomainError {
	projectsRoot := filepath.Join(loaded.Root, config.ProjectsDirName)
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return ProjectsReadDirError{Root: projectsRoot, Err: err}
	}
	for _, entry := range entries {
		// Task 0012 tracks extracting this filter into a named helper.
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var manifest project.Project
		path := filepath.Join(projectsRoot, entry.Name())
		if readErr := fsutil.ReadJSON(path, &manifest); readErr != nil {
			return readErr
		}
		if normErr := manifest.Normalize(); normErr != nil {
			return normErr
		}
		if valErr := manifest.Validate(); valErr != nil {
			return valErr
		}
		if _, exists := loaded.Projects[manifest.ID]; exists {
			return DuplicateProjectIDError{ID: manifest.ID}
		}
		loaded.Projects[manifest.ID] = &manifest
	}
	return nil
}

// ProjectList returns projects sorted by display name, which keeps the
// TUI presentation stable.
func (l *Profile) ProjectList() []*project.Project {
	var list []*project.Project
	for _, p := range l.Projects {
		list = append(list, p)
	}
	slices.SortFunc(list, func(a, b *project.Project) int {
		return strings.Compare(a.Name, b.Name)
	})
	return list
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
