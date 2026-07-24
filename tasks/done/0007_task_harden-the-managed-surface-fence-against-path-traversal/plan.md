# Plan — Harden the managed-surface fence against path traversal

Task: [./description.md](./description.md)
Guideline touched: [`docs/guidelines/security.md`](../../../docs/guidelines/security.md)
("Keep File Access Inside Intended Roots", "Test Security-Sensitive Behavior").

## Summary

Defense-in-depth only. The live write-escape is **already closed** at the sync
layer by `sync.validatePathKey` (`internal/sync/sync.go:764`), which
`filepath.Clean`s and rejects `..`/absolute keys before every `filepath.Join`.
This task closes the gap in the **outer** fence so `surfaces.IsAllowed` no
longer returns `true` for traversal strings, removes the stale `FIX: task#0007`
marker, and adds regression tests. No sync-layer code changes.

Scope is a single file plus its test: `internal/surfaces/surfaces.go` and
`internal/surfaces/surfaces_test.go`. `IsAllowed` is a **pure string
predicate** — no filesystem, subprocess, or git — so unit tests are the correct
and sufficient level (no full-stack real test trigger from
`docs/guidelines/testing.md` applies: no `filepath.Join`/`Rel`/`Abs`, no
`os/exec`, no serialization the tool didn't author).

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Managed roots are exactly this bare-name list (no trailing slash) | `internal/surfaces/surfaces.go:22` | `var roots = []string{ "AGENTS.md", ".claude", ".cursor", ".codex", ".opencode", ".mcp.json" }` |
| `IsAllowed` today matches on the raw target via `== root` or `HasPrefix(target, root+"/")`, with no cleaning | `internal/surfaces/surfaces.go:46-52` | `target = filepath.ToSlash(target); … if target == root || strings.HasPrefix(target, root+"/")` |
| The only in-repo `task#0007` link is the FIX marker on `IsAllowed` | `internal/surfaces/surfaces.go:41` | `// FIX: task#0007 — does not reject ".." segments. A target like` (grep of `internal/`+`docs/` finds no other) |
| `path` and `strings` are already imported by the file (no new import needed for `path.Clean`) | `internal/surfaces/surfaces.go:11-16` | `import ( "path"; "path/filepath"; "slices"; "strings"; "sync" )` |
| Write path already rejects `..` escapes independently of `IsAllowed` (why this is defense-in-depth, not the sole fix) | `internal/sync/sync.go:774-777` | `cleaned := filepath.ToSlash(filepath.Clean(path)); if cleaned == ".." || strings.HasPrefix(cleaned, "../") { return InvalidPathError{…"escapes project root"} }` |
| Prototype of the hardened predicate yields the intended verdict for every planned test input (all traversal→false, `.claude/./skills/x`→true) | verified in a throwaway `main` (see task session) — behavior reproduced below | `.claude/../etc/passwd→false, .claude/x/../../etc→false, /etc/passwd→false, ..\.claude\evil→false, .claude/./skills/x→true` |

## Design

Rewrite `IsAllowed` to clean the slash-normalized target first, reject any
form that escapes or is absolute, then match the cleaned form against the
managed roots:

```go
// IsAllowed reports whether target sits inside one of the managed-surface
// roots. The target is slash-normalized and path.Clean-ed first, so a
// traversal form like ".claude/../../etc/passwd" (which resolves outside
// every root) and an absolute or "../"-escaping form are all refused. A
// cleaned target is allowed only if it equals a root exactly or sits under
// one (root + "/").
func IsAllowed(target string) bool {
	if target == "" {
		return false
	}
	cleaned := path.Clean(filepath.ToSlash(target))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return false
	}
	for _, root := range roots {
		if cleaned == root || strings.HasPrefix(cleaned, root+"/") {
			return true
		}
	}
	return false
}
```

Rationale for each guard:

- `path.Clean` (slash-domain, OS-independent — not `filepath.Clean`) collapses
  `.` and resolves `..`, so `.claude/../etc/passwd` becomes `etc/passwd` and no
  longer prefix-matches `.claude/`.
- The `..` / `../` prefix check catches escapes *above* the repo root
  (`../foo`, `.claude/../../etc` → `../etc`).
- The leading-`/` check rejects absolute POSIX paths; on Windows,
  `filepath.ToSlash` first turns `..\.claude\evil` into `../.claude/evil` which
  then hits the `../` guard (on Linux the backslash form matches no root and is
  already refused). A `C:/…` volume form matches no root → refused.
- Legitimate dotted inputs (`.claude/./skills/x`) clean to a valid managed
  path and stay allowed.

The `FIX: task#0007` comment block (lines 40-45) is deleted; the doc comment is
rewritten to describe the cleaned-then-matched behavior (as above).

No change to `internal/sync/sync.go` — `validatePathKey` already provides the
write-path guard; adding a `filepath.Rel` duplicate is explicitly out of scope.

## Tests

Extend `TestIsAllowed` in `internal/surfaces/surfaces_test.go` — same
table-driven shape, add rows. Keep all existing rows (regression) and add:

| target | want | why |
|---|---|---|
| `.claude/../etc/passwd` | false | cleans to `etc/passwd`, outside roots |
| `.claude/x/../../etc` | false | cleans to `etc`, outside roots |
| `/etc/passwd` | false | absolute |
| `..\.claude\evil` | false | Windows-form escape |
| `../foo` | false | escapes above root |
| `.claude/./skills/x` | true | legitimate dotted path stays allowed |
| `""` | false | empty target |

One behavior per row; the table name already scopes the promise ("IsAllowed
returns the documented verdict"). This directly satisfies
`docs/guidelines/security.md` → "test path traversal and root-escape
rejection".

## Execution steps

1. Edit `internal/surfaces/surfaces.go`: replace the `IsAllowed` body with the
   cleaned-then-matched version above; delete the `FIX: task#0007` comment
   block and rewrite the doc comment.
2. Edit `internal/surfaces/surfaces_test.go`: add the seven rows above to the
   `TestIsAllowed` case table.
3. Run `go test ./internal/surfaces -run TestIsAllowed`.
4. Run the baseline gate `make build && make test && make lint`.
5. Confirm `grep -rn "task#0007" internal/` is empty.

## Docs / ADRs

- No ADR. This is a straight hardening of an existing invariant (CLAUDE.md
  invariant #2), not a new decision.
- No `docs/guidelines/` change.
- No arc42 change — package responsibility and boundaries are unchanged.
- Optional: a one-line changelog entry under `docs/changelog/` noting the fence
  hardening. Not required by the task; will add only if the user wants it.

## Acceptance Criteria (DoD)

- [ ] `IsAllowed` returns `false` for `.claude/../etc/passwd`,
      `.claude/x/../../etc`, `/etc/passwd`, `..\.claude\evil`, `../foo`, and
      `""`, and returns `true` for every existing valid case plus
      `.claude/./skills/x` — asserted by the extended table in `TestIsAllowed`
      (`go test ./internal/surfaces -run TestIsAllowed` green).
- [ ] `grep -rn "task#0007" internal/` returns no matches (marker removed).
- [ ] Diff touches only `internal/surfaces/surfaces.go` and
      `internal/surfaces/surfaces_test.go`; `internal/sync/sync.go` is
      unchanged (no duplicate `filepath.Rel` guard added).
- [ ] Baseline gate `make build && make test && make lint` passes.

## Out of scope

- Any change to `sync.validatePathKey` / `sync.Apply`.
- Refactoring the managed-surface root list or renaming the package.
