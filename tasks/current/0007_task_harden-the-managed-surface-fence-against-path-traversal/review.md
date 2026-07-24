# Harden the managed-surface fence against path traversal — review

The implementation meets every acceptance criterion: `IsAllowed` now
slash-normalizes and `path.Clean`s the target, rejects absolute / `..`-escaping
forms, and only then matches the managed roots; the `FIX: task#0007` marker is
gone (`grep` clean); `TestIsAllowed` gained the traversal rows; and the diff
stays inside `internal/surfaces/` with `sync.validatePathKey` untouched. Build,
test, and `go test ./internal/surfaces -run TestIsAllowed` are green.

Seven parallel reviewers (security, clean-code, clean-architecture, SOLID, DDD,
testing, Go) ran against the diff. **SOLID, DDD, and clean-architecture found no
violations** — the predicate is a correctly-layered, cohesive leaf, and the
`path.Clean` (slash-domain) vs `filepath.Clean` choice was explicitly endorsed as
the right OS-independent decision. The findings below are hardening and
coverage refinements, not blockers. Every one is safe to defer with a "leave
as-is" checkbox; the task is already correct as shipped.

The four issues, in rough priority order: (1) backslash / foreign-separator
inputs are rejected only incidentally on Linux, not by the guard, so the outer
fence does not fully "agree with the inner one" on that axis; (2) the `IsAllowed`
godoc narrates only one of the two rejection mechanisms; (3) a few
genuinely-ambiguous or branch-exercising test inputs are unpinned; (4) the new
test rows carry no documenting comments.

## Foreign-separator (backslash) inputs are rejected incidentally, not by the guard

> [!WARNING]
>
> - [security.md — "Keep File Access Inside Intended Roots"](../../../docs/guidelines/security.md#keep-file-access-inside-intended-roots)

`filepath.ToSlash` is a **no-op on Linux** (`filepath.Separator == '/'`), so a
backslash-separated traversal like `..\.claude\evil` is never split into
segments on the CI/target platform. `path.Clean` leaves it as one opaque literal
that happens to match no root, so `IsAllowed` returns `false` — but by string
mismatch, not by resolving the traversal. The task's stated goal is "harden the
outer fence so it agrees with the inner one"; on the backslash dimension it does
not. The inner guard `sync.validatePathKey` (`internal/sync/sync.go:771`)
explicitly rejects foreign separators:

```go
// sync.validatePathKey — the inner fence rejects backslashes outright:
if filepath.Separator != '/' && strings.ContainsRune(path, filepath.Separator) {
    return InvalidPathError{Path: path, Reason: "must use forward slashes"}
}

// surfaces.IsAllowed — the outer fence has no equivalent; `..\.claude\evil`
// survives path.Clean as one literal segment and is rejected only because it
// matches no root. A future root value or matcher tweak could turn an
// un-interpreted backslash segment into a match.
cleaned := path.Clean(filepath.ToSlash(target)) // ToSlash: no-op on Linux
```

This is defense-in-depth, not a live escape (the input is refused either way for
today's roots, and `validatePathKey` still guards every write). Pick one:

- [x] Reject foreign-separator inputs early to mirror `validatePathKey`: if the
      target contains a literal `\`, return `false` before cleaning. Makes the
      two fences agree and removes the platform-dependent reasoning.
- [ ] Leave as-is — the acceptance criterion (`..\.claude\evil → false`) is met,
      the write layer is authoritative, and adding the check is pure
      belt-and-suspenders. (Recommended only if you want the diff to stay
      minimal.)

## `IsAllowed` godoc narrates only one of the two rejection mechanisms

> [!WARNING]
>
> - [clean_code.md — comments must explain intent accurately](../../../docs/guidelines/clean_code.md)

The godoc (`internal/surfaces/surfaces.go:37-43`) uses
`.claude/../../etc/passwd` as the exemplar and says traversal forms "resolve
outside every root" and "are all refused". That describes only the explicit
`../`-prefix guard. The traversal form the tests actually exercise —
`.claude/../etc/passwd` (test line 27) — cleans to `etc/passwd`, which does
**not** trip the `../` guard at all; it is refused solely by failing to match any
root in the loop. A reader debugging why `etc/passwd` is rejected will look at
the guard, not find it there, and be confused. There are two independent
defenses (escape-prefix guard + root membership on the cleaned path) and the doc
narrates one.

```go
// Current godoc implies the guard rejects all traversal:
//   ...a traversal form like ".claude/../../etc/passwd" (which resolves
//   outside every root) ... are all refused.
// But ".claude/../etc/passwd" -> "etc/passwd" is refused by ROOT MISMATCH,
// not by the "../" guard.
```

- [x] Reword the godoc to state both mechanisms: forms that clean to an
      absolute path or a `../`-escape are rejected outright; anything else must
      still match a root exactly or by `root + "/"` prefix. Optionally add one
      line noting that a `..`-bearing input is permitted when it re-resolves
      inside a root (e.g. `.claude/../.claude/x`).
- [ ] Leave as-is — the behavior is correct; only the explanation is lossy.

## A few genuinely-ambiguous or branch-exercising test inputs are unpinned

> [!WARNING]
>
> - [security.md — "Test Security-Sensitive Behavior"](../../../docs/guidelines/security.md#test-security-sensitive-behavior)
> - [testing.md — smallest useful test / pin the contract](../../../docs/guidelines/testing.md)

The seven added rows cover the primary attack forms, and the testing reviewer
confirmed no full-stack real test is owed (the predicate does no
`filepath.Join`/`Rel`/`Abs` and no I/O — a unit table is the correct level). But
a handful of security-relevant cases are unpinned, most notably the **re-entrant
traversal** `.claude/../.claude/x`, which is the one input whose intended verdict
is genuinely non-obvious. It currently returns `true` (cleans to `.claude/x`) —
correct and safe, but exactly the contract a future refactor could silently
break. Separately, the explicit `cleaned == ".."` branch (`surfaces.go:49`) is
never hit directly by the table (`../foo` exercises the `HasPrefix("../")` arm
instead).

```go
// Candidate additions to the TestIsAllowed table:
{".claude/../.claude/x", true},  // escape + re-enter cleans to .claude/x — stays inside fence
{"..", false},                   // hits the `cleaned == ".."` branch directly
{".claude/..", false},           // cleans to "." — root traverses back to cwd
{".", false},                    // cwd itself is not a managed surface
{".claude/", true},              // trailing slash normalizes to the bare root
{`C:\evil`, false},              // Windows volume form (incidental rejection today)
{`\\host\share\x`, false},       // UNC form
```

- [x] Add the re-entrant row plus the `".."` / `".claude/.."` / `"."` rows (pins
      the ambiguous contract and the currently-unexercised equality branch).
      (Recommended — cheap, closes the sharpest gap.)
- [ ] Add all of the above including the trailing-slash and Windows/UNC
      defense-in-depth anchors.
- [ ] Leave as-is — the acceptance-criteria cases are covered and the extra
      inputs already return the right answer.

## New traversal test rows carry no documenting comments

> [!WARNING]
>
> - [clean_code.md — test data intent should be visible](../../../docs/guidelines/clean_code.md)

The seven new rows (`internal/surfaces/surfaces_test.go:27-33`) encode the whole
point of task 0007 — traversal, absolute, empty, `.`-normalization — but sit
anonymously among the pre-existing happy-path rows with no grouping comment or
per-row rationale. Why `.claude/./skills/x → true` matters (it proves
`path.Clean` doesn't over-reject a benign `.` segment) is not legible from the
tuple alone, and `..\.claude\evil → false` holds for a platform-dependent reason
worth signalling.

```go
{".claude/./skills/x", true},  // benign "." segment must survive Clean, not be rejected
{`..\.claude\evil`, false},    // Windows-style separators: rejected on POSIX by root-mismatch
{"", false},                   // empty target guarded explicitly
```

- [x] Add a short comment per traversal row (or a `// --- path traversal ---`
      divider before them) so the security intent is legible.
- [ ] Leave as-is — the `%q` in the failure message already names the offending
      input, so the table is arguably self-evident.
