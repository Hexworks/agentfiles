# 0030 changes

Adds a **Register as Asset** action to the **Plan Project** screen. When the
cursor sits on a directory row whose every descendant file is `? unknown`, a
new `r` button opens the existing Create Asset modal pre-filled with the
folder's name. On submit the folder is turned into a profile-owned asset: a new
asset is created under the active profile, the folder's files are **copied**
into the profile's `assets/`, and the asset is selected for the current
project. The plan then reloads in place so those files re-classify from
`? unknown` to managed `+ add` / `~ update` rows. The original project files are
left untouched.

The work reuses existing seams end-to-end: a recursive copy helper in `utils`,
a folder constructor in `asset`, a bundled use case in `app` that keeps the
create-copy-select invariant inside one consistency boundary, a thin `actions`
forwarder, and the modal lifecycle pattern already used by the Edit Profile
screen.

## Decisions

- The folder's absolute source path is carried on the screen
  (`registerSourceDir`), not in `asset.Manifest` — **Why:** the manifest has no
  source field and gains no durable reason to grow one; the path is transient
  UI state for one confirm cycle.
- `CreateAssetFromFolder` is a single bundled use case rather than reusing
  `InitAsset` + `SelectAsset` — **Why:** the create-then-copy-then-select steps
  form one invariant; splitting them risks a half-registered asset, and
  `SelectAsset` validates against a `loaded.Assets` map that predates the new
  asset.
- `asset.json` is written **last** in `InitFromFolder`, after the copy —
  **Why:** a stray `asset.json` in the source folder must not clobber the
  manifest we generate.
- **Register** is offered on any directory whose every descendant file is
  unknown (ancestors of an all-unknown subtree qualify too) — **Why:** the user
  picks the right folder; over-restricting to leaf-most dirs would block valid
  choices. Directories with any managed leaf are excluded because they are
  already partly owned.
- Success keeps the user on the screen and reloads (dedicated
  `registerAssetDoneMsg`) rather than reusing `mutationCmd`, which pops —
  **Why:** the point of registering is to see the re-classified plan.

Not done (out of scope per task): changing how `ChangeUnknown` is classified;
moving or deleting the original project files.

## Assumptions

- Symlinks in the source folder are skipped (only regular files are copied) —
  **Why:** asset content is plain files; following symlinks risks escaping the
  source tree.

## Other Notes

- Documented the `r` action in `docs/manual/plan_project.md`.
- No ADR: reuses existing patterns (modal lifecycle, app use case, managed-surface
  fence); no new durable architecture decision.
- Tests added: `utils.CopyDir`, `asset.InitFromFolder`,
  `app.Service.CreateAssetFromFolder`, `dirAllUnknown`, the dir-row register
  button, and `afterRegisterAsset` success → reload.

## utils.CopyDir

New recursive copy helper plus a typed `CopyDirError`. Walks the source tree,
copies each regular file preserving its mode and relative structure (parent
dirs auto-created by `WriteFile`), and skips non-regular entries.

```go
// after
func CopyDir(src, dst string) errs.DomainError {
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel := ToRelative(src, path)
		if writeErr := WriteFile(filepath.Join(dst, filepath.FromSlash(rel)), data, info.Mode().Perm()); writeErr != nil {
			return writeErr
		}
		return nil
	})
	if walkErr != nil {
		return CopyDirError{Src: src, Dst: dst, Err: walkErr}
	}
	return nil
}
```

## asset.InitFromFolder

Folder-backed asset constructor: validate manifest → compute the standard
`assets/<type>/<id>/` dir → copy the source folder in → write `asset.json` last.

```go
// after
func InitFromFolder(root string, manifest Manifest, sourceDir string) (string, errs.DomainError) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)
	if err := utils.EnsureDir(dir); err != nil {
		return "", err
	}
	if err := utils.CopyDir(sourceDir, dir); err != nil {
		return "", err
	}
	if err := utils.WriteJSON(filepath.Join(dir, config.AssetManifestFileName), manifest); err != nil {
		return "", err
	}
	return dir, nil
}
```

## app.Service.CreateAssetFromFolder

Bundled use case: resolve the project, derive the id, dup-check, build the
asset from the folder, then append it to the project's selection and persist.

```go
// after
func (s *Service) CreateAssetFromFolder(profileRef, projectID string, manifest asset.Manifest, sourceDir string) (string, errs.DomainError) {
	loaded, p, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return "", err
	}
	if manifest.ID == "" {
		manifest.ID = utils.Slug(manifest.Name, config.DefaultAssetSlug)
	}
	if loaded.Assets[manifest.ID] != nil {
		return "", AssetExistsError{AssetID: manifest.ID}
	}
	if _, initErr := asset.InitFromFolder(loaded.Root, manifest, sourceDir); initErr != nil {
		return "", initErr
	}
	p.SelectedAssetIDs = append(p.SelectedAssetIDs, manifest.ID)
	if saveErr := project.Save(loaded.Root, p); saveErr != nil {
		return "", saveErr
	}
	return manifest.ID, nil
}
```

## plan_project screen wiring

The screen gained a modal (`modal *modal.Modal` + `planModalKind`), a
`registerSourceDir`, and window-size tracking. `treeActionsFn` now returns a
`[Register]` button on directory rows where `dirAllUnknown` holds; confirming
the modal runs `CreateAssetFromFolder` and reloads the plan via a dedicated
`registerAssetDoneMsg` (no pop).

```go
// after
func (s *planProjectScreen) treeActionsFn() treetable.ActionsFunc {
	return func(n *treetable.Node) []*mnemonic.Button {
		if d, ok := planDirNode(n); ok {
			if dirAllUnknown(n) {
				return []*mnemonic.Button{s.registerAssetBtn(d.path)}
			}
			return nil
		}
		// ... file-row buttons unchanged
	}
}
```
