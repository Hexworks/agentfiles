---
id: 9001
type: feature
status: in-review
topics: go
---

# Fixture: happy path

Adds a `Greeter` helper that returns `"hello, <name>"` and a companion test.
This fixture exists to exercise Step 6.5 end-to-end: every criterion is met
by the accompanying diff, and every hunk in the diff traces to a criterion.

## Acceptance Criteria

- [ ] `internal/greet.Greeter.Hello(name)` returns `"hello, <name>"` for a
      non-empty `name` — `TestGreeterHello`.
- [ ] `internal/greet.Greeter.Hello("")` returns the empty string and does
      not panic — `TestGreeterHelloEmpty`.

## Out of scope

- Localization / i18n of the greeting text.
- Any TUI wiring of the greeter.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/greet -run 'TestGreeterHello|TestGreeterHelloEmpty'` green.

## Plan

[plan.md](./plan.md)
