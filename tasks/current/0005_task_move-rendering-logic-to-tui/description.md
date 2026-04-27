---
id: 0005
type: task
status: Pending
---

# Move all rendering logic to the TUI module

There are places in the code where we have rendering logic. These are all marked by `// FIX:` sections.
This is a remnant of the previous CLI setup, but now we have a dedicated TUI module which is responsible for rendering.

All places where have rendering should be moved to the TUI. In these places we should just create metadata instead
that will be used for rendering, for example if the code looks like this:

```go
fmt.Fprintf(&b, "- [%s] %s: %s\n", change.Kind, change.Path, change.Reason)
```

we should just return the data that we'll use for rendering instead:

```go
return &Metadata{ Kind: change.Kind, Path: change.Path, Reason: change.Reason}
```

That we'll use in the TUI for rendering.

Make sure that whenever errors are returned that are not strings the following pattern is used:

```go
// This is the error metadata
type CustomError struct {
    IntField int
    StrField string
}

// The error metadata implements the `Error()` function that returns a `string` representation
func (e CustomError) Error() string {
    return fmt.Sprintf("shit happened: %d, %s", e.IntField, e.StrField)
}
```
