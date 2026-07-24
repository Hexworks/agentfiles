# asset-init-strategy review

Task 0010 replaces the hard-coded `writeStarter`/`RequiredContentFile` `Type`
switches with a single `starters` map + `//go:embed` `text/template` files, all
kept in-package. The refactor is clean and lands its intent: byte-identical
output (verified against on-disk hexdumps), one source of truth for
`Type → (filename, template)`, no new import edges, and no touch to the
out-of-scope `render` switch (task 0011). Build, `go vet`, and the 33 asset
tests all pass.

Security, clean-architecture, DDD, and Go-idiom reviews returned **no
findings**. The remaining items are two genuine test-coverage gaps and two minor
clean-code nits — none block acceptance; all are optional polish. Pick one
checkbox per issue (including the "leave as-is" option where offered).

Explicitly dropped during synthesis (not issues): the SOLID "split `starters`
into two maps" suggestion (it would reintroduce the exact filename/template
drift this task exists to kill — the paired struct is correct by design); the
SOLID "OCP: `AllTypes()`/`Manifest.Validate()` are still switches" note (those
switches are out of scope — the task scopes only `writeStarter` +
`RequiredContentFile`); and the testing "expected bytes are copies of a
constant" note (the literals are authored independently and the bytes come from
the template, so a template drift makes the test _fail_ — that is the correct
pattern, not a violation).

## `StarterRenderError` path has no test

> [!WARNING]
>
> - [testing.md](docs/guidelines/testing.md) — a declared error branch with no test asserting it

`renderStarter` wraps a template-execution failure in `StarterRenderError`
(`internal/asset/starter.go:50-52`, type in `internal/asset/errors.go:112-131`),
but no test exercises that branch. The templates are static and embedded, so the
branch is effectively unreachable at runtime today — which is exactly why it is
easy to let it rot: a future template edit that references a missing `Manifest`
field would surface here, and there is no test to prove the error is typed,
carries the right `Type`, and unwraps.

```go
// starter.go — the untested branch
func renderStarter(s starter, manifest Manifest) ([]byte, errs.DomainError) {
	var buf bytes.Buffer
	if err := starterTmpl.ExecuteTemplate(&buf, s.template, manifest); err != nil {
		return nil, StarterRenderError{Type: manifest.Type, Err: err} // no test reaches here
	}
	return buf.Bytes(), nil
}
```

Choose one:

- [x] Add a small unit test on `renderStarter` (package-internal `_test.go`) that
      passes a `starter` whose `template` names a template referencing a
      non-existent field (or an unparseable action), and assert the result is a
      `StarterRenderError` with the expected `Type` and a non-nil `Unwrap()`.
- [ ] Leave the branch untested but add a one-line comment at the wrap site
      stating the path is unreachable with the current embedded templates and is
      kept as a guard for future template edits (documents the deliberate gap).

## No test freezes the `text/template` (non-)escaping contract

> [!WARNING]
>
> - [testing.md](docs/guidelines/testing.md) — behavioral contract (escaping) left unpinned

The skill/agents_doc starters interpolate `Manifest.Name`/`Description` via
`text/template`, which — unlike `html/template` — performs **no** escaping. That
non-escaping is the intended behavior (these are markdown/SKILL files, not
HTML), but nothing pins it: a future well-meaning switch to `html/template`
would silently turn a description containing `<`, `>`, `&`, or `"` into
`&lt;`/`&amp;`/… and no test would catch it. Current tests only use benign
alphanumeric values (`Name: "Rev"`, `Description: "desc"`).

```go
// asset_test.go — only the benign case is covered
dir, err := Init(root, Manifest{ID: "rev", Name: "Rev", Type: TypeSkill, Description: "desc"})
want := "---\nname: Rev\ndescription: desc\n---\n\nDescribe the skill here.\n"
// nothing asserts that Description: `a < b & "c"` stays literal (unescaped)
```

Choose one:

- [x] Extend `TestInit_SkillStarter_*` (or add one case) with a `Description`
      containing `<`, `>`, `&`, `"` and assert the bytes on disk contain those
      characters **verbatim**, freezing the no-escaping contract.
- [ ] Leave as-is — accept that the escaping contract is implied by the
      `text/template` import and not separately pinned.

## Four near-identical `TestInit_*Starter` tests duplicate setup

> [!WARNING]
>
> - [clean_code.md](docs/guidelines/clean_code.md) — repeated setup/read/assert boilerplate across tests

`TestInit_SkillStarter_InterpolatesNameAndDescription`, `TestInit_AgentsDocStarter`,
and `TestInit_SettingsStarter_IsStatic` (`internal/asset/asset_test.go:293-335`)
share the same four-line shape: `t.TempDir()` → `Init` → `os.ReadFile(join(dir, file))`
→ byte-compare. Only the manifest, filename, and expected bytes vary. This is a
readability/maintenance nit, not a correctness problem — explicit tests are also
a defensible project style, so this is genuinely optional.

```go
// three tests, one shape — only inputs/outputs differ
root := t.TempDir()
dir, err := Init(root, Manifest{ID: "rev", Name: "Rev", Type: TypeSkill, Description: "desc"})
// ...err check...
got, readErr := os.ReadFile(filepath.Join(dir, "SKILL.md"))
// ...err check...
if string(got) != want { t.Fatalf(...) }
```

Choose one:

- [x] Extract a helper `initAndReadStarter(t, manifest, filename) []byte` used by
      the three tests, keeping each test to its manifest + expected bytes.
- [ ] Fold the three convention-type cases into one table-driven test
      (`{typ, manifest, filename, want}` rows), matching the style already used by
      `TestInit_GenericTypes_WriteNoStarterFile` and
      `TestRequiredContentFile_DerivesFromStrategy`.
- [ ] Leave as-is — keep the three tests explicit for individually readable
      failure output.

## `starter.template` restates the `.tmpl` extension and its comment restates usage

> [!WARNING]
>
> - [clean_code.md](docs/guidelines/clean_code.md) — mild redundancy in field value and its comment

Minor. The `template` field stores the full base filename including `.tmpl`
(`"skill.md.tmpl"`), and its comment (`// base name inside templates/, e.g.
"skill.md.tmpl"`) largely restates how the field is used a few lines below in
`renderStarter`. Neither is wrong; both are small polish opportunities.

```go
// starter.go
type starter struct {
	filename string // written into the asset dir (a config.*StarterFileName)
	template string // base name inside templates/, e.g. "skill.md.tmpl"
}
var starters = map[Type]starter{
	TypeSkill: {config.SkillStarterFileName, "skill.md.tmpl"}, // ".tmpl" repeated per entry
	// ...
}
```

Choose one:

- [ ] Leave as-is — the explicit full filename is unambiguous and matches the
      real file on disk 1:1 (recommended: lowest risk, clearest mapping).
- [x] Store the bare stem (`"skill.md"`) and append `.tmpl` in `renderStarter`,
      and tighten the field comment to state the invariant (must match a parsed
      template) rather than the usage.
