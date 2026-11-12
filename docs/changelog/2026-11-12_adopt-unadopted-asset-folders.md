# Adopt unadopted asset folders

Adds an **Adopt** action (`o`) to the **Edit Profile** screen. A profile often
gains assets by someone copying a skill folder straight into
`assets/skill/`; such a folder has no `asset.json`, so `profile.Load` skips it
and the Assets panel never shows it. The screen now also lists these
*unadopted folders* on every load, renders a one-line hint under the Assets
panel when any exist, and lets the user onboard one: a picker over the
folders, then the regular Create Asset form (seeded with the folder's name and
type, type select restricted to the folder-registerable types) with the same
validation the Register-as-Asset flow applies. On confirm the manifest is
written in place; if the chosen id or type puts the asset's home elsewhere the
folder is renamed to `assets/<type>/<id>/`. Nothing is copied and no repo is
touched.

## Decisions

- Scan lives in `profile` (`ScanUnadopted`), not in `asset` — **Why:** the
  profile package already owns walking the `assets/` tree for `Load`; the
  scan is the complement of that walk.
- Only `asset.FolderRegisterableTypes()` directories are scanned —
  **Why:** generic types (mcp/rule/hook) need projections the Create Asset
  form does not collect, so a bare folder there could not be adopted anyway.
  Hidden directories are skipped.
- The TUI hands the service an assets-root-relative key (`<type>/<name>`),
  and the service re-scans and re-asserts it (`FolderNotAdoptableError`) —
  **Why:** mirrors `CreateAssetFromFolder`'s `DirKey` contract; a stale or
  tampered key cannot adopt an arbitrary directory.
- `asset.AdoptFolder` renames instead of copying and refuses an existing
  target (`AssetDirExistsError`) — **Why:** the content is already inside the
  profile; copying would leave a duplicate, and overwriting would clobber a
  sibling asset.
- Mnemonic is `o` ("Ad**o**pt") — **Why:** `a` is taken by Select Assets on
  the Projects panel and the mnemonic set is unique per screen.

Considered but deliberately not done:

- Loading unadopted folders as placeholder assets — would make render see
  manifest-less assets and blur invariant #6.
- Auto-adopting with defaults — the user asked for the regular onboarding
  (validation, compatible agents, etc.), so every adoption goes through the
  form.
