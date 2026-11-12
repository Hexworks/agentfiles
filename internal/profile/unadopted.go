package profile

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
)

// UnadoptedFolder is a directory that sits where an asset would live
// (`assets/<type>/<name>/`) but carries no asset.json, so Load skips it.
// Typical origin: a skill folder copied straight into the profile. The
// Adopt flow turns it into a real asset by writing the manifest (and
// moving the folder when the chosen id or type differs from the folder's
// current location).
type UnadoptedFolder struct {
	// Key is the forward-slash path relative to the assets root
	// (`<type>/<name>`). It is the handle the TUI hands back to the
	// service, which re-resolves it against the profile root rather than
	// trusting an absolute path from the caller.
	Key string
	// Type is derived from the parent directory name.
	Type asset.Type
	// Name is the folder's basename; the Adopt form seeds the asset name
	// from it.
	Name string
	// Dir is the absolute on-disk path.
	Dir string
}

// ScanUnadopted lists the direct children of every folder-registerable
// type directory under <root>/assets that have no asset.json. Only the
// types in asset.FolderRegisterableTypes are scanned because those are
// the only types whose content can be adopted from a bare folder; the
// generic types need projections the Adopt form does not collect.
// Hidden directories (leading dot) and non-directories are skipped. A
// missing type directory contributes nothing. Results are sorted by Key.
func ScanUnadopted(root string) ([]UnadoptedFolder, errs.DomainError) {
	root, absErr := utils.ToAbsolute(root)
	if absErr != nil {
		return nil, absErr
	}
	assetsRoot := filepath.Join(root, config.AssetsDirName)
	var out []UnadoptedFolder
	for _, t := range asset.FolderRegisterableTypes() {
		typeDir := filepath.Join(assetsRoot, string(t))
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, AssetsScanError{Root: typeDir, Err: err}
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dir := filepath.Join(typeDir, e.Name())
			if utils.Exists(filepath.Join(dir, config.AssetManifestFileName)) {
				continue
			}
			out = append(out, UnadoptedFolder{
				Key:  path.Join(string(t), e.Name()),
				Type: t,
				Name: e.Name(),
				Dir:  dir,
			})
		}
	}
	slices.SortFunc(out, func(a, b UnadoptedFolder) int {
		return strings.Compare(a.Key, b.Key)
	})
	return out, nil
}
