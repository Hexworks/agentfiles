# Go Guidelines

Go code in `agentfiles` should make the domain explicit, keep side effects easy to
find, and keep errors actionable. The codebase is small enough that simple
packages and direct data structures are usually better than abstraction-heavy
frameworks.

The best default is to keep domain logic separate from command handling and file
I/O. Packages should describe the problem space clearly: registry, profile,
asset, project, render, and sync each exist because they model different parts
of the system.

## Keep Packages Cohesive

Packages should represent one clear domain concern.

```go
// Do: keep project validation inside the project package.
func (m *Manifest) Validate() error {
	if m.ID == "" || m.Name == "" || m.Path == "" {
		return fmt.Errorf("project id, name, and path are required")
	}
	return nil
}
```

```go
// Don't: spread project validation through unrelated command handlers.
if projectID == "" || name == "" || path == "" {
	return errors.New("bad input")
}
```

## Prefer Explicit Types Over Loose Maps

Use structs when the domain has stable shape and meaning.

```go
// Do
type ManagedState struct {
	ProfileID        string            `json:"profile_id"`
	ProjectID        string            `json:"project_id"`
	ManagedFiles     map[string]string `json:"managed_files"`
}
```

```go
// Don't
state := map[string]any{
	"profile_id": profileID,
	"files":      files,
}
```

## Prefer Structs Over Tuples

When a group of values travels together — as a function's return, a
parameter list, or a field — name it. **Two values may stay a tuple; three
or more must be a named struct.** With two values the order is usually
obvious (`value, err`, `key, value`, `x, y`); with three or more, callers
can no longer tell which position means what without jumping to the
definition, and every added value multiplies the risk of a silent
argument-order swap.

Idiomatic Go pairs are exempt regardless: `(T, error)`, `(value, ok)`,
`(index, found)`. These are language conventions and stay as-is.

```go
// Do: name the result once, reuse it everywhere.
type RenderPlan struct {
	Files   []FileChange
	Ignored []string
	Deletes []string
}

func Build(p *profile.Profile, proj *project.Manifest) (RenderPlan, errs.DomainError) {
	...
}
```

```go
// Don't: three positional returns the caller has to decode by position.
func Build(...) ([]FileChange, []string, []string, errs.DomainError) {
	...
}
```

The same rule applies to parameters — an options struct once the arguments
pass three — and to composite literals, where a repeated anonymous
`struct{...}` with three fields should be promoted to a named type. A
short-lived two-field anonymous struct (a table-test row, a quick grouping
inside one function) is fine; name it the moment it grows a third field or
escapes the function.

A named struct also documents itself (`AddProjectArgs{Path: p}` says what
`p` is), cannot be swapped by accident, survives a new field without a
breaking signature change, and can grow methods (`Validate`, `Normalize`,
`Error`) where a tuple cannot.

## Route With A Switch, Not An If-Else Chain

When control flow selects between **more than two** branches on the same
value, use a `switch`. Two branches may stay an `if`/`else`; the moment a
third branch appears, promote it to a `switch`. A switch names the value
being routed on once, keeps the cases aligned and easy to scan, and makes a
missing case obvious.

```go
// Do: three-way routing reads as a switch.
switch change.Kind {
case ChangeCreate:
	return applyCreate(change)
case ChangeUpdate:
	return applyUpdate(change)
case ChangeDelete:
	return applyDelete(change)
default:
	return UnknownChangeError{Kind: change.Kind}
}
```

```go
// Don't: an if-else ladder hides the routed value and the missing default.
if change.Kind == ChangeCreate {
	return applyCreate(change)
} else if change.Kind == ChangeUpdate {
	return applyUpdate(change)
} else if change.Kind == ChangeDelete {
	return applyDelete(change)
}
```

Prefer a `default` case that handles the unexpected value — usually by
returning a typed error — so a new variant can't slip through silently.

## Persist Only Validated, Versioned Data

Every document that is written to disk and read back is a schema with a
lifetime longer than any single build of the program. Treat persistence as
a boundary, not a convenience:

- **Every persisted document carries a `version`.** An integer envelope
  field is enough. It is what lets a future build tell an old file from a
  new one.
- **Migration and validation run at one boundary, not per caller.** The
  load and save helpers — not each call site — run *migrate then validate*.
  A caller that cannot skip validation cannot forget it. Prefer a type
  constraint so "this type is persistable" is compiler-checked.
- **A missing version is a legacy sentinel, not an error.** A document
  written before versioning decodes its `version` to the zero value.
  Migration stamps it up to the current version so existing files keep
  loading; it never downgrades.
- **Reject a version newer than this build knows.** If a document's version
  exceeds the highest the running build understands, refuse to load it. An
  old binary that silently rewrites a newer file into an older shape is
  data loss. Fail loudly instead.
- **Validate before the write touches the filesystem.** An invalid value
  must never reach disk — no partial file, no stray temp file, no created
  directory.

```text
Do:
- give each on-disk type a version field and a migrate + validate pair
- run migrate-then-validate inside the shared read/write helpers
- treat version 0 as "legacy, migrate up"; treat version > known as "reject"
```

```text
Don't:
- unmarshal straight into a struct and trust it
- scatter Validate() calls that each caller must remember
- rename or overload an existing version field that already means
  something else — add a second one if the concerns differ
```

Repo specifics: the boundary is `utils.ReadJSON` / `utils.WriteJSON*`,
generic over the `utils.Persisted[T]` constraint (`*T` with `Migrate()`,
`SchemaVersion()`, and `Validate()`). The boundary runs the reject-newer
guard once from `SchemaVersion()` (returning the shared
`errs.NewerSchemaVersionError`), so `Validate()` covers only each type's
domain shape and may be a no-op for envelope-only types. See
[ADR 0022](../adr/0022-validated-versioned-persistence-boundary.md).
The one sanctioned exception is `internal/migrate`, which reads
pre-versioning legacy shapes raw (they are transient migration DTOs, not
`Persisted` types).

## Return Actionable Errors

Errors should explain what failed and why the caller should care. For
domain failures the preferred form is a typed error struct rather than an
ad-hoc `fmt.Errorf` string — see [`errors.md`](./errors.md) for the full
convention, including loop accumulation via `errors.Join`.

```go
// Do
return AssetNotFoundError{ID: id}
```

```go
// Don't
return errors.New("not found")
```

## Keep I/O At The Edges

File reads and writes should stay close to packages whose job is persistence or
synchronization, not inside unrelated domain coordination code.

```go
// Do: render computes outputs, sync writes them.
plan, err := render.Build(profile, project)
err = sync.Apply(preview, deleteCandidates)
```

```go
// Don't: hide arbitrary file writes inside selection logic.
func selectAssets(...) {
	_ = os.WriteFile(path, data, 0o644)
}
```

## Use Behavior-Focused Tests

Tests should verify domain rules, not just increase line count.

```go
// Do: assert ownership conflicts and drift semantics.
if _, err := svc.AddProject("second", "Repo2", projectPath, []string{"codex"}, nil); err == nil {
	t.Fatal("expected ownership conflict")
}
```

```go
// Don't: rely only on smoke tests that miss business rules.
func TestService(t *testing.T) {}
```

## Implement the `Error` function whenever non-string values are returned as errors

If a function returns with an error (eg: `(result, CustomError)`) make sure that the struct that
is returned also has an `Error` function that can turn it into a `string`.

```go
type CustomError struct {
    IntField int
    StrField string
}

func (e CustomError) Error() string {
    return fmt.Sprintf("shit happened: %d, %s", e.IntField, e.StrField)
}
```

**Note that** this guideline only applies if a simple `string` is not sufficient.
