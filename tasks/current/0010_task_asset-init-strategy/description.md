---
id: 0010
type: task
status: pending
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
