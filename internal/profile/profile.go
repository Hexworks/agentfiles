// Package profile represents a profile folder on disk: its manifest, the
// assets it contains, and the projects it owns. It is the authoritative source
// of truth that render and sync consume.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/addamsson/agentfiles/internal/asset"
	"github.com/addamsson/agentfiles/internal/config"
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
	Projects map[string]*project.Manifest
}

// Init scaffolds a brand-new profile root with the expected folder layout.
// The created directories mirror the current set of first-class asset types.
func Init(root, name string) (*Manifest, error) {
	root, err := fsutil.ToAbsolute(root)
	if err != nil {
		return nil, err
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
func Load(root string) (*Profile, error) {
	root, err := fsutil.ToAbsolute(root)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := fsutil.ReadJSON(filepath.Join(root, config.ProfileManifestFileName), &manifest); err != nil {
		return nil, err
	}
	loaded := &Profile{
		Root:     root,
		Manifest: manifest,
		Assets:   map[string]*asset.Asset{},
		Projects: map[string]*project.Manifest{},
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
func scanAssets(loaded *Profile) error {
	assetsRoot := filepath.Join(loaded.Root, config.AssetsDirName)
	return filepath.WalkDir(assetsRoot, func(path string, d os.DirEntry, err error) error {
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
		a, err := asset.Load(path)
		if err != nil {
			return err
		}
		if _, exists := loaded.Assets[a.ID]; exists {
			// FIX: use error struct instead of strings @see task#0005
			return fmt.Errorf("duplicate asset id: %s", a.ID)
		}
		loaded.Assets[a.ID] = a
		return filepath.SkipDir
	})
}

// scanProjects loads all project manifests from the profile's projects/
// directory and normalizes them before exposing them to the rest of the app.
func scanProjects(loaded *Profile) error {
	projectsRoot := filepath.Join(loaded.Root, config.ProjectsDirName)
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var manifest project.Manifest
		path := filepath.Join(projectsRoot, entry.Name())
		if err := fsutil.ReadJSON(path, &manifest); err != nil {
			return err
		}
		if err := manifest.Normalize(); err != nil {
			return err
		}
		if err := manifest.Validate(); err != nil {
			return err
		}
		if _, exists := loaded.Projects[manifest.ID]; exists {
			// FIX: use error struct instead of string @see task#0005
			return fmt.Errorf("duplicate project id: %s", manifest.ID)
		}
		loaded.Projects[manifest.ID] = &manifest
	}
	return nil
}

// ProjectList returns projects sorted by display name, which keeps CLI/TUI
// presentation stable.
func (l *Profile) ProjectList() []*project.Manifest {
	var list []*project.Manifest
	for _, p := range l.Projects {
		list = append(list, p)
	}
	slices.SortFunc(list, func(a, b *project.Manifest) int {
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
