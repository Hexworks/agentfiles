---
id: 0010
type: task
status: in-review
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

## Acceptance Criteria

Boundary-crossing criteria exercise **real** `asset.Init` against the real
filesystem in `t.TempDir()` and assert bytes on disk (not a constant the code
also produced). Expected strings are the user-visible contract, authored
independently of the template files.

- [ ] `make fmt && make test && make lint && make build` all pass.
- [ ] `TestInit_SkillStarter_InterpolatesNameAndDescription`: real `asset.Init`
      into `t.TempDir()` with `Name:"Rev", Description:"desc"`, reads
      `assets/skill/<id>/SKILL.md` off disk, asserts exact bytes
      `"---\nname: Rev\ndescription: desc\n---\n\nDescribe the skill here.\n"`.
- [ ] `TestInit_AgentsDocStarter`: real `Init`, reads `AGENTS.md`, asserts
      `"# <name>\n"`.
- [ ] `TestInit_SettingsStarter_IsStatic`: real `Init`, reads `codex.toml`,
      asserts `"# codex settings\n"`.
- [ ] `TestInit_GenericTypes_WriteNoStarterFile`: for each of `mcp`, `rule`,
      `hook`, real `Init` produces a dir containing **only** `asset.json` (no
      starter file), asserted by listing the on-disk dir.
- [ ] `TestRequiredContentFile_DerivesFromStrategy`:
      `skill`/`agents_doc`/`settings` return their `config.*StarterFileName` +
      `true`; `mcp`/`rule`/`hook` return `("", false)`.
- [ ] Existing `TestFolderRegisterableTypes_AreConventionTypesOnly` and
      `TestInitFromFolder_*` still pass unchanged (no behavior regression).
- [ ] Single source: `starters` map is the only `Type → (filename, template)`
      table; the old `writeStarter` and `RequiredContentFile` switches are
      deleted.

## Out of scope

- The `render` per-`(Type, Agent)` switch (`internal/render/render.go`) —
  tracked by task 0011.
- Adding any new asset type.
- Moving `asset.Type` to a leaf package.

## Verification

1. Run `make fmt && make test && make lint && make build` — all green.
2. `go test ./internal/asset/...` — the five new `TestInit_*` /
   `TestRequiredContentFile_*` tests pass alongside the pre-existing
   `TestFolderRegisterableTypes_*` / `TestInitFromFolder_*`.
3. `grep -n "switch" internal/asset/asset.go` — no `Type`-switch remains in
   `writeStarter` or `RequiredContentFile` (both are `starters`-map lookups).
4. `ls internal/asset/templates/` shows `skill.md.tmpl`, `agents_doc.md.tmpl`,
   `settings.toml.tmpl`, embedded via `//go:embed` in `starter.go`.
