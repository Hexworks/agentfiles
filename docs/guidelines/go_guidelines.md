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

## Return Actionable Errors

Errors should explain what failed and why the caller should care.

```go
// Do
return fmt.Errorf("selected asset not found: %s", id)
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
