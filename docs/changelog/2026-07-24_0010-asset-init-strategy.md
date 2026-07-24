# 0010 changes

Replaced `asset.Init`'s hard-coded per-`Type` starter `switch` (inline body
bytes) with a data-driven strategy table keyed by `asset.Type`. Each entry owns
a starter filename and an **embedded** `text/template`; templates ship inside
the binary via `//go:embed templates`. The second `Type`-switch,
`RequiredContentFile`, now derives from the same table, so the written starter
filename and the required-content filename can no longer drift apart.

Everything lives inside the `asset` package (new `starter.go` + `templates/`),
avoiding an import cycle a subpackage would create on `asset.Type`/`Manifest`.
Behavior is **byte-identical** to before: skill/agents_doc/settings emit the
same starter content; the generic types (`mcp`/`rule`/`hook`) still emit no
starter file (no table entry → `nil`). No `default` branch is needed because
`Manifest.Validate` already rejects unknown types upstream.

## Decisions

- Keep the strategy in-package (`starter.go`) rather than a subpackage —
  **Why:** a subpackage needs `asset.Type`/`Manifest` and `asset.Init` must call
  it, which cycles. "Strategy package" read as the strategy *pattern*, per the
  approved clarification.
- Consolidate `RequiredContentFile` and `FolderRegisterableTypes` onto the same
  `starters` map — **Why:** one source for `Type → filename` kills the drift
  risk between the written filename and the required-content filename.
- Use `text/template` executed against the `Manifest` — **Why:** the skill and
  agents_doc starters interpolate `Name`/`Description`; templates keep the body
  declarative and out of Go string concatenation.
- No ADR — **Why:** internal cohesion refactor; the managed-surface/render
  invariants are untouched.

Considered but not done: the `render` per-`(Type, Agent)` switch
(`internal/render/render.go`) is out of scope — tracked by task 0011.

## Assumptions

- Starter bytes must stay byte-identical (including trailing newlines) —
  **Why:** the task frames this as an extraction refactor, not a content change;
  tests freeze the exact bytes as the contract.

## Other Notes

- `docs/`: extended the `asset` package bullet in `CLAUDE.md` to describe the
  embedded starter-template table. No guideline or arc42 change.
- New tests exercise the real `asset.Init` + real filesystem in `t.TempDir()`
  and assert against bytes on disk, not against a constant the code also
  produced.

## `writeStarter` → table lookup

```go
// before
func writeStarter(dir string, manifest Manifest) errs.DomainError {
	switch manifest.Type {
	case TypeSkill:
		body := []byte("---\nname: " + manifest.Name + "\ndescription: " + manifest.Description + "\n---\n\nDescribe the skill here.\n")
		return utils.WriteFile(filepath.Join(dir, config.SkillStarterFileName), body, 0o644)
	case TypeAgentsDoc:
		return utils.WriteFile(filepath.Join(dir, config.AgentsDocStarterFileName), []byte("# "+manifest.Name+"\n"), 0o644)
	case TypeSettings:
		return utils.WriteFile(filepath.Join(dir, config.SettingsStarterFileName), []byte("# codex settings\n"), 0o644)
	}
	return nil
}
```

```go
// after — look up the type's starter, render its embedded template, write it.
// No entry (mcp/rule/hook) → no starter file.
func writeStarter(dir string, manifest Manifest) errs.DomainError {
	s, ok := starters[manifest.Type]
	if !ok {
		return nil
	}
	body, err := renderStarter(s, manifest)
	if err != nil {
		return err
	}
	return utils.WriteFile(filepath.Join(dir, s.filename), body, 0o644)
}
```

## `RequiredContentFile` → derives from the same table

```go
// before
func RequiredContentFile(t Type) (string, bool) {
	switch t {
	case TypeSkill:
		return config.SkillStarterFileName, true
	case TypeAgentsDoc:
		return config.AgentsDocStarterFileName, true
	case TypeSettings:
		return config.SettingsStarterFileName, true
	}
	return "", false
}
```

```go
// after — single source of truth; filename can't drift from what's written
func RequiredContentFile(t Type) (string, bool) {
	s, ok := starters[t]
	return s.filename, ok
}
```

## New `starter.go` — embedded templates + strategy table

```go
// after — new file: embed FS, pre-parsed templates, the strategy table, renderer
//go:embed templates
var starterTemplates embed.FS

var starterTmpl = template.Must(template.ParseFS(starterTemplates, "templates/*.tmpl"))

type starter struct {
	filename string // written into the asset dir (a config.*StarterFileName)
	template string // base name inside templates/, e.g. "skill.md.tmpl"
}

var starters = map[Type]starter{
	TypeSkill:     {config.SkillStarterFileName, "skill.md.tmpl"},
	TypeAgentsDoc: {config.AgentsDocStarterFileName, "agents_doc.md.tmpl"},
	TypeSettings:  {config.SettingsStarterFileName, "settings.toml.tmpl"},
}

func renderStarter(s starter, manifest Manifest) ([]byte, errs.DomainError) {
	var buf bytes.Buffer
	if err := starterTmpl.ExecuteTemplate(&buf, s.template, manifest); err != nil {
		return nil, StarterRenderError{Type: manifest.Type, Err: err}
	}
	return buf.Bytes(), nil
}
```
