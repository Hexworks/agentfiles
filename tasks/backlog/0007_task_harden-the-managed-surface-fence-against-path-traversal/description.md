---
id: 0007
type: task
status: pending
topics: security, go
---

# Harden the managed-surface fence against path traversal

`surfaces.IsAllowed` (`internal/surfaces/surfaces.go`) accepts a target if it
equals one of the managed roots or has a `root + "/"` prefix. It does not
clean the target first, so a projection like `.claude/../../etc/passwd`
satisfies the prefix check and is then handed to `filepath.Join` in
`sync.Apply` (`internal/sync/sync.go`), which writes outside the project root.

CLAUDE.md's invariant #2 frames the managed-surface fence as the system's
guarantee that writes stay inside the managed surface. Today the fence only
enforces a prefix.

## Scope

- Tighten `surfaces.IsAllowed` to reject targets containing `..` segments,
  leading `/`, or any volume prefix; or `filepath.Clean` the target and
  verify the cleaned form still has the same managed root and contains no
  `..`.
- Add a defense-in-depth check in `sync.Apply`: after
  `filepath.Join(projectPath, rel)` compute `filepath.Rel(projectPath, abs)`
  and refuse to write if the result starts with `..`.
- Extend `internal/surfaces/surfaces_test.go` with traversal cases:
  `.claude/../etc/passwd`, `.claude/x/../../etc`, `/etc/passwd`,
  `..\\.claude\\evil` (Windows form).
- Remove the `FIX: task#0007` marker on `surfaces.IsAllowed` once both
  defenses are in place.

## Out of scope

- Refactoring the managed-surface root list itself.
- Renaming the package.
