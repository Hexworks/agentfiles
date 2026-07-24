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
