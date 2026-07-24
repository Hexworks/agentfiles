---
id: 0010
type: task
status: in-progress
topics: go, asset
---

# Replace `asset.Init` switch with per-Type strategy + templates

`asset.Init` (`internal/asset/asset.go`) hard-codes the starter file for
each `Type` in a switch with inline body bytes. Adding a new asset type
requires editing this switch and editing several other switches across
the codebase.

Introduce a strategy package keyed by `asset.Type`, with each strategy
owning its own template files. Use `embed` to ship template content
inside the binary (see `../../../agentfiles-old/` for an example layout
with `embed.FS`).

Originally tracked as a `// TODO:` block in `asset.go`.

## Clarification

### Question

How should the per-Type starter strategy be packaged, given that `asset.Init`
must call it and a naive subpackage would cycle on `asset.Type`/`Manifest`?

### Answer

Keep the strategy **inside the `asset` package** (Option A): a new `starter.go`
plus a `//go:embed templates/` directory. Cycle-free, cohesive, no new import
edges. "Strategy package" is read as the strategy *pattern*, not a separate Go
package.

### Question

`RequiredContentFile` is a second `Type`-switch returning each type's starter
filename. Fold it into the same strategy table?

### Answer

Yes — consolidate both. The strategy table becomes the single source for
`Type → (filename, template)`. `RequiredContentFile` and
`FolderRegisterableTypes` derive from it, deleting the second switch and the
drift risk between the written filename and the required-content filename.

### Question

How should starter content that interpolates `Name`/`Description` (skill,
agents_doc) be rendered?

### Answer

`text/template`: embed `.tmpl` files and execute them against the `Manifest`.
Behavior must stay byte-identical to today's inline starters. Generic types
(mcp, rule, hook) have no strategy entry and produce no starter file.
