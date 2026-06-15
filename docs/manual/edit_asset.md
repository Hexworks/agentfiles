# Edit Asset

Edit one asset's files and metadata. Left pane is a file tree under
`assets/<asset>/`; right pane holds the editable manifest fields.

## Focus order

`tab` / `shift+tab` cycle through:

1. Files tree
2. Description
3. Tags
4. Compatible Agents
5. Exclusive Group

Mnemonic shortcuts (`o`, `d`, `a`, `e`, `b`) only fire while the **tree** is
focused. In any text input, those letters type as characters.

## Tree row actions (tree focus only)

- **Open** (`o`) — open the highlighted file in `$EDITOR`. Files only.
- **Delete** (`d`) — remove the highlighted file or directory.

## Screen actions (tree focus only)

- **Add** (`a`) — modal to create a new file inside the asset.
- **Save** (`e`) — persist the manifest fields (Description / Tags / Compatible
  Agents / Exclusive Group).
- **Back** (`b` / `esc`) — return to **Edit Profile**; unsaved changes prompt
  a confirm.

## Notes

`Compatible Agents` empty means "all enabled agents". `Exclusive Group` is a
mutual-exclusion key — two assets with the same group cannot both be selected
on a single project.
