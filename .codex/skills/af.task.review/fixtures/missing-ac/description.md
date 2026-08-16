---
id: 9002
type: feature
status: in-review
topics: go
---

# Fixture: missing Acceptance Criteria

Deliberately omits `## Acceptance Criteria` so `af.task.review` Step 6.5a
fails fast with the `LegacyTask` outcome — no diff read, no Step 6.5b/c,
no Step 7 dispatch.

## Out of scope

- none

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/oops -run TestOopsNoop` green.

## Plan

[plan.md](./plan.md)
