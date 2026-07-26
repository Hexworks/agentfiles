---
id: 0043
type: task
status: Pending
topics: sync_and_safety, testing, documentation
depends_on: 0035, 0001
---

# Schema-migration guardrails (post-mortem from 0035)

Follow-up from task 0035 review. Task 0035 introduced a state.json v2→v3
schema bump (`ManagedFileEntry` with `AssetID`/`SourceRel` provenance) and
declared v2 legacy tolerance with "Adopt disabled until the next re-apply
repopulates them". The implementation of `preserveDriftBaseline` in
`internal/sync/sync.go:428-442` copies the whole prior entry on drift-Keep,
which prevents v2 legacy entries from ever migrating to v3 for drifted
paths — the exact opposite of the description's intent. Users hit
`AdoptUnavailableError` even after applying the same project multiple
times.

Root causes (four, all preventable):

1. **Description contradicted plan.** Description said "next re-apply
   repopulates them" (upgrade); plan Step 5 said "the drift-Keep and
   drift-Adopt branches preserve the prior baseline entry (whole
   `ManagedFileEntry`)" (freeze). Reviewer + implementor missed
   contradiction.
2. **ADR 0015 pattern got copy-pasted through a type widening.** Original
   DriftKeep stored a bare `Hash` string; schema v3 widened the value type
   to a struct. Author swapped `string → ManagedFileEntry` mechanically
   without per-field intent audit.
3. **Test coverage stopped at the single-apply happy path.** Every named
   acceptance-criterion test asserts one Apply's behaviour;
   `TestApply_DriftAdopt_LegacyV2Entry_ReturnsAdoptUnavailable` pins the
   error case, not the migration case.
4. **Review skill missed it.** Six subagents + a DoD gate; none simulated
   Apply N+1 state.

## Scope

Deliver three guardrails so this class of bug is caught before merge:

- **Schema-migration checklist** appended to
  `docs/guidelines/sync_and_safety.md` requiring a per-field intent audit
  (preserve / upgrade / drop) whenever a persisted value type widens.
  Every branch that copies a prior entry must justify each field's
  choice in a code comment tied to the checklist.
- **DoD rule**: any state-schema bump (detected by change to
  `GeneratorVersion` const or `ManagedFileEntry`-style struct) requires
  a `TestX_LegacyEntry_MigratesOnNextApply` test that runs Apply twice
  and asserts the second state.json carries the new provenance.
- **Review dimension**: add a "schema-lifecycle" subagent to
  `af.task.review` that traces one entry through N applies for tasks
  whose diff modifies `internal/sync/sync.go` `ManagedFileEntry`,
  `GeneratorVersion`, or `loadState`. The subagent's prompt asks
  explicitly: "if a user runs Apply, then Apply again with the same
  input, what does the state file look like at N+2?"
- **af.task.plan skill enhancement**: when the plan's DoD contradicts an
  Out-of-scope bullet in the description, surface the contradiction as
  a clarifying question during grilling. The specific pattern to detect
  is a description phrase like "until X repopulates them" paired with a
  plan step that preserves prior state verbatim.

## Out of scope

- Fixing the underlying v2→v3 preservation bug in `internal/sync/sync.go`
  — handled by the `preserveDriftBaseline freezes legacy v2 entries
forever` issue in `tasks/current/0035_feature_adopt-drift-into-profile/review.md`
  and resolved by `af.task.review-apply 35`.
- Broader task-workflow overhaul; each guardrail is a minimal addition.
- Retroactive schema migration tooling for v2 entries — description
  0035 already declared this out of scope.

## Acceptance Criteria

- [ ] `docs/guidelines/sync_and_safety.md` gains a "Schema evolution"
      section with the per-field preserve/upgrade/drop checklist and a
      worked example citing the 0035 bug.
- [ ] `af.task.implement` DoD gate rejects a schema-bump diff (grep for
      `GeneratorVersion` const value change or a new field on a persisted
      struct in `internal/sync/`) unless a `Test*_Migrates*` test exists
      in the same commit.
- [ ] `af.task.review` skill definition adds a "schema-lifecycle" review
      subagent, dispatched only when `git diff --name-only` includes
      `internal/sync/sync.go`. The subagent's prompt requires an
      Apply-N+1 trace and reports back either `NO_FINDINGS` or specific
      per-field failures.
- [ ] `af.task.plan` skill definition adds a "contradiction check" step
      after grilling: parse the description and the plan for
      "until X repopulates" / "preserve prior entry" (or equivalent)
      pairings; surface any hit as a mandatory clarifying question.
- [ ] Manual verification: replay the 0035 bug against the four
      guardrails and confirm each would have caught it (documented in a
      changelog entry).

## Verification

- `make build && make test && make lint` (baseline gate).
- `grep -n "Schema evolution" docs/guidelines/sync_and_safety.md`
  returns the new section.
- Read the updated `af.task.review` skill file end-to-end; the
  schema-lifecycle subagent appears in the Step 7 topic list with a
  dispatch condition.
- Read the updated `af.task.plan` skill file end-to-end; the
  contradiction-check step appears with the exact grep patterns.
- Dry-run: run `af.task.review 35` against a synthetic version of the
  0035 diff without the `preserveDriftBaseline` fix; the
  schema-lifecycle subagent flags it.
