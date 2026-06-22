# Plan — 0034 Definition of Done + acceptance criteria

See [description.md](./description.md) for motivation + acceptance criteria.

## Goal

Cut intent→output drift by giving every task an explicit Definition of Done and
making `af.task.review` refuse work that doesn't meet it — without verbose spec
tooling and without spawning an extra review subagent (token economy).

Design principle — **one section, double duty:** the `## Acceptance Criteria`
hybrid checklist *is* the DoD. A box `[x]` = that slice done. No separate DoD
section to restate it. Done = every AC box `[x]` + `## Verification` passes.

## Part 1 — `af.create-task` mandates the DoD

File: `.claude/skills/af.create-task/SKILL.md`

- **Layout facts + Step 6:** stop emitting `status: active` (invalid —
  implement/review accept only `pending|in-progress|blocked|in-review|done`).
  Current-folder tasks start `pending`; implement flips to `in-progress`.
- **Step 7:** template now writes three required body sections after the title —
  `## Acceptance Criteria` (verifiable checklist), `## Out of scope`,
  `## Verification`. Document that the AC checklist is the DoD.
- **Step 8:** grilling must fill them; interview cannot finish until AC has ≥1
  verifiable box and Out-of-scope is present (`- none` allowed). Removed stray
  `Lofasz` junk line.
- **Examples:** `example-1-description.md` + `example-2-description.md` gained a
  real `## Acceptance Criteria` block so they model the convention.

## Part 2 — `af.task.review` Definition-of-Done gate

File: `.claude/skills/af.task.review/SKILL.md`

- **New Step 6.5 — Definition-of-Done Gate (mandatory).** Inserted after Step 6
  (the `git diff` is in hand) and before Step 7 (subagent dispatch). Placement
  is the point: orchestrator-only, no subagents, so a fail short-circuits the
  expensive agent burst.
  - AC missing/empty → stop ("predates convention").
  - Each criterion judged met/unmet from the diff with `file:line` evidence.
  - Scope creep: any diff hunk tracing to no criterion (and not justified by
    changelog / Out-of-scope) → finding.
  - All met + no creep → continue to Step 7; else stop, report gaps, no dispatch.
- **Stopping-Conditions table:** added rows for missing AC and unmet/creep.

## Files

| File | Change |
| --- | --- |
| `.claude/skills/af.create-task/SKILL.md` | mandate 3 sections; fix `status: active`; grilling gate; drop junk |
| `.claude/skills/af.create-task/example-1-description.md` | add `## Acceptance Criteria` |
| `.claude/skills/af.create-task/example-2-description.md` | add `## Acceptance Criteria` |
| `.claude/skills/af.task.review/SKILL.md` | add Step 6.5 gate + stopping rows |

## Notes

- No `.codex/` or `~/.agentfiles/` mirrors of these skills exist — `.claude/`
  copies are authoritative (verified via `find`).
- `af.task.implement` deliberately untouched (kept scope to the two parts).
