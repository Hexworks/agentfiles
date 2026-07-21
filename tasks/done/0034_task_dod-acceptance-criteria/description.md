---
id: 0034
type: task
status: done
topics: documentation
---

# Definition of Done + acceptance criteria to reduce intent→output drift

The repo carries recurring `drift:` commits — gaps between what was wanted and
what the workflow produced. Two root causes: (1) `description.md` has no required
Definition of Done, so the agent stops at "looks done"; (2) `af.task.review`
checks craftsmanship via 7 subagents but never checks that *what was asked got
built and nothing extra crept in*.

Fix in two parts (token-minimal, no new spec tooling, no extra review subagent):

1. **`af.create-task`** mandates a `## Acceptance Criteria` hybrid checklist
   (the checklist *is* the DoD) + `## Out of scope` + `## Verification`. Grilling
   fills them; can't finish with an empty/vague checklist.
2. **`af.task.review`** gets a **Step 6.5 Definition-of-Done Gate** that runs on
   description + diff *before* dispatching the subagents — checks each criterion
   met-from-diff and flags scope creep. Fail → stop before the agent burst.

## Acceptance Criteria

- [ ] `af.create-task` Step 7 template writes `## Acceptance Criteria`,
      `## Out of scope`, `## Verification` into every new `description.md`.
- [ ] `af.create-task` Step 8 blocks grilling from finishing until AC has ≥1
      verifiable checkbox and Out-of-scope is filled.
- [ ] `af.create-task` no longer writes invalid `status: active` — current-folder
      tasks start `pending` (implement flips to `in-progress`).
- [ ] Both example description files carry a real `## Acceptance Criteria` block.
- [ ] `af.task.review` has a Step 6.5 gate, placed after Step 6 (diff read) and
      before Step 7 (dispatch), that stops without dispatching when any criterion
      is unmet, scope creep exists, or `## Acceptance Criteria` is missing/empty.
- [ ] `af.task.review` Stopping-Conditions table lists the new gate failures.

## Out of scope

- No new `af.task.review` subagent (8th agent) — gate is inline to save tokens.
- No EARS / Given-When-Then / Spec-Kit-style multi-file specs.
- No `af.task.implement` change (no plan↔AC coverage check added).
- No Go source changes — workflow/skill-doc only.

## Verification

- Baseline: `make build && make test && make lint` pass.
- Inspection: `git diff --stat` covers the SKILL files, both examples, the
  glossary, the ADR, the changelog, the arc42 building-block view, task
  0036's frontmatter, and the three fixture directories under
  `.claude/skills/af.task.review/fixtures/`.
- Fixture `passes/`: reviewer walks `description.md` + `diff.patch`, Step
  6.5a passes, 6.5b table rows all `met`, 6.5c every hunk resolves via
  (1) — expected message: _"6.5 gate passed → dispatching Step 7
  subagents."_ (or equivalent; the point is Step 7 fires).
- Fixture `missing-ac/`: reviewer reads `description.md`, Step 6.5a fails
  fast — expected stop message: _"LegacyTask — task is missing the required
  '## Acceptance Criteria' section per the task-workflow contract. Add a
  verifiable checklist before review can run."_ No diff read, no 6.5b/c,
  no Step 7 dispatch.
- Fixture `scope-creep/`: reviewer reads `description.md` + `diff.patch`,
  6.5a passes, 6.5b's single row is `met`, 6.5c flags
  `internal/log/verbose.go:1` as unresolved — expected stop message:
  _"6.5c: 1 unresolved hunk — internal/log/verbose.go:1 (no criterion,
  no refactor-allowed bullet, no changelog mechanical-follow-up). Not
  dispatching Step 7 subagents."_

## Plan

[plan.md](./plan.md)
