# 0007 changes

Hardened the outer managed-surface fence so `surfaces.IsAllowed` no longer
returns `true` for path-traversal strings. Previously `IsAllowed` matched the
raw target against each managed root via `== root` or `HasPrefix(target,
root+"/")` without cleaning it first, so a form like `.claude/../../etc/passwd`
satisfied the prefix check. The predicate now slash-normalizes and
`path.Clean`s the target, rejects absolute and `..`-escaping forms, then matches
the cleaned form against the roots.

This is defense-in-depth cleanup, not a live-exploit fix: the concrete
write-escape was already closed one layer down by `sync.validatePathKey`, which
cleans and rejects `..`/absolute keys before every `filepath.Join`. A malicious
target passed `IsAllowed` but was refused at `writeRendered`, so no file was
ever written outside the project root. This change makes the outer fence agree
with the inner one and removes the stale `FIX: task#0007` marker that pointed
at the gap.

The regression test `TestIsAllowed` gained traversal rows (all expecting
`false`) plus a legitimate dotted case (`.claude/./skills/x` → `true`).

## Decisions

- Clean with `path.Clean` (slash domain), not `filepath.Clean` — **Why:**
  `IsAllowed` is a pure OS-independent string predicate over forward-slash keys;
  `path.Clean` gives deterministic behavior regardless of host OS, matching how
  render/sync build project-relative keys.
- Left `sync.validatePathKey` / `sync.Apply` untouched; added no duplicate
  `filepath.Rel` guard — **Why:** the write-path defense is already correct and
  broader; the task explicitly scopes the diff to `internal/surfaces/`.

## Assumptions

- The only in-repo `task#0007` reference was the FIX marker on `IsAllowed` —
  **Why:** confirmed by `grep -rn "task#0007" internal/` returning nothing after
  the marker was deleted.

## Other Notes

- No ADR, guideline, or arc42 change — this is a straight hardening of CLAUDE.md
  invariant #2, not a new decision. Package responsibility and boundaries are
  unchanged.
- No full-stack real test is required (per `docs/guidelines/testing.md`):
  `IsAllowed` performs no `filepath.Join`/`Rel`/`Abs`, no `os/exec`, and no I/O,
  so a table-driven unit test is the correct and sufficient level.

## Harden `surfaces.IsAllowed` and drop the FIX marker

```go
// before
// FIX: task#0007 — does not reject ".." segments. A target like
// ".claude/../../etc/passwd" passes the prefix check today; tighten by
// cleaning the target and rejecting any escape from the managed root.
func IsAllowed(target string) bool {
	target = filepath.ToSlash(target)
	for _, root := range roots {
		if target == root || strings.HasPrefix(target, root+"/") {
			return true
		}
	}
	return false
}
```

```go
// after — clean the slash-normalized target, refuse absolute/escaping forms,
// then match the cleaned form against the managed roots
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

## Extend `TestIsAllowed` with traversal cases

```go
// after — added rows to the existing case table
{".claude/../etc/passwd", false}, // cleans to etc/passwd, outside roots
{".claude/x/../../etc", false},   // cleans to etc, outside roots
{"/etc/passwd", false},           // absolute
{`..\.claude\evil`, false},       // Windows-form escape
{"../foo", false},                // escapes above root
{".claude/./skills/x", true},     // legitimate dotted path stays allowed
{"", false},                      // empty target
```
