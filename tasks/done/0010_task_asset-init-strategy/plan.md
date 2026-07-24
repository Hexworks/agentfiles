# Plan — 0010: Replace `asset.Init` switch with per-Type strategy + templates

Task: [`./description.md`](./description.md)
Touches: `internal/asset/asset.go`, `internal/asset/errors.go`, new
`internal/asset/starter.go`, new `internal/asset/templates/`,
`internal/asset/asset_test.go`.

## Goal

Replace the hard-coded `writeStarter` `switch` (inline body bytes per
`asset.Type`) with a data-driven strategy table keyed by `asset.Type`. Each
entry owns a starter filename and an **embedded** `text/template`. Fold the
second `Type`-switch (`RequiredContentFile`) into the same table so the
per-type filename lives in exactly one place. Behavior stays **byte-identical**
to today.

Scope note: the `render` type-switch (`internal/render/render.go:137`) is
explicitly **out of scope** — it is tracked by task 0011. This task touches
only the asset-scaffolding path.

## Design decisions (from Clarification)

1. **Placement — inside `asset`.** Strategy lives in `internal/asset/starter.go`
   with a `//go:embed templates` directory in the same package. A separate
   subpackage would cycle: strategy needs `asset.Type`/`Manifest`, and
   `asset.Init` must call the strategy. Keeping it in-package is cycle-free and
   cohesive (matches the "be pragmatic / start simple" guidelines).
2. **Consolidate filename source.** The strategy table is the single source for
   `Type → (filename, template)`. `RequiredContentFile` and
   `FolderRegisterableTypes` derive from it; the second switch is deleted.
3. **`text/template`.** Templates are embedded `.tmpl` files, pre-parsed once
   via `template.Must(template.ParseFS(...))`, executed against `Manifest`.

## Target shape

```
internal/asset/
  asset.go        // writeStarter + RequiredContentFile delegate to the table
  starter.go      // NEW: embed FS, parsed templates, strategy table, renderStarter
  errors.go       // + StarterRenderError
  templates/      // NEW
    skill.md.tmpl
    agents_doc.md.tmpl
    settings.toml.tmpl
```

```go
// starter.go
//go:embed templates
var starterTemplates embed.FS

var starterTmpl = template.Must(template.ParseFS(starterTemplates, "templates/*.tmpl"))

// starter describes how a scaffolded asset of a Type seeds its initial content.
type starter struct {
    filename string // written into the asset dir (a config.*StarterFileName)
    template string // base name inside templates/, e.g. "skill.md.tmpl"
}

var starters = map[Type]starter{
    TypeSkill:     {config.SkillStarterFileName, "skill.md.tmpl"},
    TypeAgentsDoc: {config.AgentsDocStarterFileName, "agents_doc.md.tmpl"},
    TypeSettings:  {config.SettingsStarterFileName, "settings.toml.tmpl"},
}
// TypeMCP/TypeRule/TypeHook: no entry -> no starter file (user-authored).
```

Template contents must reproduce current bytes exactly:

- `skill.md.tmpl`:
  ```
  ---
  name: {{.Name}}
  description: {{.Description}}
  ---

  Describe the skill here.
  ```
  (single trailing `\n`)
- `agents_doc.md.tmpl`: `# {{.Name}}\n`
- `settings.toml.tmpl`: `# codex settings\n` (no interpolation)

`writeStarter` and `RequiredContentFile` collapse to table lookups; the generic
types fall out naturally as "no entry → nil / (\"\", false)". No `default`
branch needed — `Manifest.Validate` already rejects unknown types upstream.

## Execution steps

1. **Create `internal/asset/templates/`** with the three `.tmpl` files above.
   Get the skill/agents_doc bytes exactly right (trailing newlines).
2. **Add `internal/asset/starter.go`**: embed directive, parsed-templates var,
   `starter` struct, `starters` map, `renderStarter(s starter, m Manifest)
   ([]byte, errs.DomainError)` that executes the named template into a
   `bytes.Buffer` and wraps any execute error in `StarterRenderError`.
3. **Add `StarterRenderError{Type Type; Err error}`** to `errors.go` with
   `Error()` and `Severity() errs.SeverityError` (mirrors existing typed
   errors). Covers the rare template-execute failure.
4. **Rewrite `writeStarter`** (`asset.go`) to look up `starters[manifest.Type]`;
   on miss return `nil`; on hit `renderStarter` then
   `utils.WriteFile(filepath.Join(dir, s.filename), body, 0o644)`. Remove the
   stale `// task 0010 tracks…` comment.
5. **Rewrite `RequiredContentFile`** to derive from `starters`
   (`s, ok := starters[t]; return s.filename, ok`). Delete the old switch.
   `FolderRegisterableTypes` is unchanged (still iterates `AllTypes()` via
   `RequiredContentFile`).
6. **Tests** (see Acceptance Criteria) — extend `asset_test.go`.
7. **Docs**: update the `asset` bullet in `CLAUDE.md` to mention the embedded
   starter-template table; remove the obsolete task-0010 TODO reference. No ADR
   (internal cohesion refactor, no durable cross-cutting decision).
8. **Gate**: `make fmt && make test && make lint && make build`.

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Asset dir layout is `assets/<type>/<id>/` | `internal/asset/asset.go:202` | `dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)` |
| `writeStarter` is the switch to replace; only skill/agents_doc/settings emit a file | `internal/asset/asset.go:224-233` | `switch manifest.Type { case TypeSkill: … case TypeAgentsDoc: … case TypeSettings: … } return nil` |
| Current skill starter bytes | `internal/asset/asset.go:226-227` | `body := []byte("---\nname: " + manifest.Name + "\ndescription: " + manifest.Description + "\n---\n\nDescribe the skill here.\n")` |
| Current agents_doc starter bytes | `internal/asset/asset.go:229` | `utils.WriteFile(filepath.Join(dir, config.AgentsDocStarterFileName), []byte("# "+manifest.Name+"\n"), 0o644)` |
| Current settings starter bytes (static) | `internal/asset/asset.go:231` | `utils.WriteFile(filepath.Join(dir, config.SettingsStarterFileName), []byte("# codex settings\n"), 0o644)` |
| `RequiredContentFile` is the 2nd Type→filename switch to fold in | `internal/asset/asset.go:242-252` | `case TypeSkill: return config.SkillStarterFileName, true …` |
| Starter filenames: `SKILL.md`, `AGENTS.md`, `codex.toml` | `internal/config/paths.go:65,70,74` | `const SkillStarterFileName = "SKILL.md"` / `AgentsDocStarterFileName = "AGENTS.md"` / `SettingsStarterFileName = "codex.toml"` |
| `Init` seeds via `writeStarter`; callers are `app.Service` | `internal/asset/asset.go:171-175`, `internal/app/service.go:207` | `return asset.Init(loaded.Profile.Root, manifest)` |
| `Manifest.Validate` rejects unknown types → no `default` needed | `internal/asset/asset.go:133-137` | `default: return UnsupportedAssetTypeError{Type: m.Type}` |
| render Type-switch is a **separate** task (0011), out of scope | `internal/render/render.go:137` | `// Task 0011 tracks replacing this switch with a per-(Type, Agent) strategy lookup.` |
| `//go:embed` can only embed files at/under the `.go` file's dir | Go spec — `embed` package (external) | — |
| `template.ParseFS(fs, "templates/*.tmpl")` names each template by base filename | `text/template` / `html/template` stdlib docs (external) | — |

## ADRs / docs

- **No ADR.** Internal refactor; the managed-surface/render invariants are
  untouched.
- **CLAUDE.md**: extend the `asset` package bullet to note the embedded
  starter-template strategy table (single source for `Type → filename +
  template`).
- No changes to `docs/guidelines/`.

## Acceptance Criteria

Every boundary-crossing criterion below exercises **real** `asset.Init` + the
real filesystem in `t.TempDir()` and asserts against bytes on disk (not against
a constant the code also produced). The expected strings are authored as the
user-visible contract, independent of the template files.

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
      starter file) — asserted by listing the on-disk dir.
- [ ] `TestRequiredContentFile_DerivesFromStrategy`: `skill`/`agents_doc`/
      `settings` return their `config.*StarterFileName` + `true`;
      `mcp`/`rule`/`hook` return `("", false)`.
- [ ] Existing `TestFolderRegisterableTypes_AreConventionTypesOnly` and
      `TestInitFromFolder_*` still pass unchanged (no behavior regression).
- [ ] Byte-identity guard: rendered skill/agents_doc/settings bytes match the
      pre-refactor output (the three asserted literals above are the frozen
      contract).

## Out of scope

- The `render` per-`(Type, Agent)` switch — task 0011.
- Adding any new asset type.
- Moving `asset.Type` to a leaf package.
