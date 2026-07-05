---
id: 9003
type: feature
status: in-review
topics: go
---

# Fixture: scope-creep

Ships the same `Greeter` as the `passes/` fixture, but the diff also drops
a brand-new `internal/log/verbose.go` file that is unrelated to the stated
criteria, is not covered by a refactor-allowed bullet in `description.md`,
and is not named as a mechanical follow-up in the changelog. Step 6.5c
must flag the extra file as an unresolved hunk.

## Acceptance Criteria

- [ ] `internal/greet.Greeter.Hello(name)` returns `"hello, <name>"` for a
      non-empty `name` — `TestGreeterHello`.

## Out of scope

- Logging / observability changes.
- Any TUI wiring of the greeter.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/greet -run TestGreeterHello` green.

## Plan

[plan.md](./plan.md)
