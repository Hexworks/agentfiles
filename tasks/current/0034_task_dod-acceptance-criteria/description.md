---
id: 0034
type: task
status: in-review
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

```
# Inspection-based (process change, not buildable):
git -C . diff --stat   # 4 files: af.create-task SKILL + 2 examples, af.task.review SKILL
```

- Dry-run `af.create-task`: generated `description.md` has the 3 sections + valid status.
- `af.task.review` happy path: AC met → Step 6.5 passes → subagents dispatch.
- `af.task.review` fail path: unmet AC or stray hunk → stops at 6.5, no dispatch.
- Back-compat: in-review task lacking `## Acceptance Criteria` → "predates convention" stop.

## Plan

[plan.md](./plan.md)
