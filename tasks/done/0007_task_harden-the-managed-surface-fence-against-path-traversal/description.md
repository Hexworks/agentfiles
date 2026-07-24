---
id: 0007
type: task
status: done
topics: security, go, sync_and_safety
notes: Original write-escape vuln is already closed at the sync layer by validatePathKey (task 0006 write-path guards). Remaining work is hardening IsAllowed itself + removing its stale FIX marker + tests. See Background.
---

# Harden the managed-surface fence against path traversal

`surfaces.IsAllowed` (`internal/surfaces/surfaces.go`) accepts a target if it
equals a managed root or has a `root + "/"` prefix. It does **not** clean the
target first, so a projection target like `.claude/../../etc/passwd` satisfies
the prefix check. CLAUDE.md invariant #2 frames the managed-surface fence as
the guarantee that writes stay inside the managed surface; today the outer
fence only enforces a prefix.

## Background

Since this task was first filed, the concrete write-escape is **already closed
one layer down**. `sync.validatePathKey` (`internal/sync/sync.go:764`)
`filepath.Clean`s every path key and rejects `..` / `../` escapes before any
`filepath.Join`. It guards `writeRendered`, the adopt entries, `loadState`
keys, and the ignored-paths list, and is re-exported as `sync.ValidatePathKey`
for `app.DiffFile`. A malicious `.claude/../../etc/passwd` passes `IsAllowed`
but is refused at `writeRendered` → no file is written outside the project
root.

So this task is **no longer a live exploit** — it is defense-in-depth cleanup:
`IsAllowed` still returns `true` for traversal strings, and the source still
carries a `FIX: task#0007` marker pointing here. Harden the outer fence so it
agrees with the inner one, and delete the stale marker.

The originally-proposed second defense (a `filepath.Rel` check inside
`sync.Apply`) is **dropped** — `validatePathKey` already provides it, more
broadly. Do not add a duplicate.

## Scope

- Tighten `surfaces.IsAllowed` so it refuses any target that is absolute,
  carries a volume prefix, or escapes its managed root: `filepath.Clean` the
  (slash-normalized) target and return `false` unless the cleaned form still
  sits at-or-under the same root and contains no `..` segment.
- Remove the `FIX: task#0007` comment block on `IsAllowed` once the guard is
  in place.
- Extend `internal/surfaces/surfaces_test.go` `TestIsAllowed` with traversal
  cases (all expecting `false`) plus one legitimate dotted case that must stay
  `true`.

## Acceptance Criteria

- [ ] `IsAllowed` returns `false` for every traversal input
      (`.claude/../etc/passwd`, `.claude/x/../../etc`, `/etc/passwd`,
      `..\.claude\evil`) and still returns `true` for the existing valid cases
      plus `.claude/./skills/x` — asserted by an extended `TestIsAllowed`
      (`go test ./internal/surfaces -run TestIsAllowed`).
- [ ] No `task#0007` marker remains in the source —
      `grep -rn "task#0007" internal/` returns nothing.
- [ ] The write-path traversal defense stays `validatePathKey`, unchanged; no
      duplicate `filepath.Rel` guard is added to `sync.Apply` — the diff
      touches only `internal/surfaces/`.

## Out of scope

- Refactoring the managed-surface root list itself.
- Renaming the `surfaces` package.
- Any change to `sync.validatePathKey` or `sync.Apply` — the write-path
  defense is already correct.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/surfaces -run TestIsAllowed` green with the new
  traversal cases.
- `grep -rn "task#0007" internal/` prints nothing.
