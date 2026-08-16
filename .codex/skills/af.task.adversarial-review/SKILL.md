---
name: Adversarial Review [Agentfiles]
description: Use when the user invokes /adversarial-review <task-number> (e.g. /adversarial-review 0001) to stress-test an already-planned task before implementation. Reads the approved plan.md, juxtaposes it against the current architecture (docs/architecture/) and ADRs (docs/adr/), hunts for holes — over-engineering, scope creep, redundancy, contradictions with existing decisions — then conducts a one-question-at-a-time adversarial interview via the `grilling` skill to trim the plan down to only what is necessary. Edits plan.md in place; keeps status `in-progress`. Runs between /plan-task and /implement-task. Project-specific to repos that follow the tasks/{backlog,current,done}/ convention with directory-per-task layout.
---

# Adversarial Review

Stress-tests an approved `plan.md` **before** implementation. Takes a task id (e.g. `0001`), validates it is in `in-progress` with a `plan.md`, gathers the current architecture and ADRs, dispatches adversarial subagents to find holes, then runs a relentless one-at-a-time interview (via the `grilling` skill) to challenge every part of the plan against what already exists.

The north star is **necessity**. The goal is **not** to change the plan for its own sake — a plan that survives review unchanged is a good outcome. The goal is that whatever remains in the plan is *necessary*: nothing over-engineered, nothing already covered by an existing pattern/ADR, nothing outside the task's scope, nothing that fights the current architecture. Bias every challenge toward **cutting**, not adding.

This skill sits between planning and implementation:

```
/plan-task  →  /adversarial-review  →  /implement-task
```

> [!IMPORTANT]
> This skill **only** refines `plan.md` and its `description.md` Q&A record. It does **not** touch production code and does **not** change task status.

> [!IMPORTANT]
> Follow the steps **in order**. Do not skip a step.

## Task Layout

Each task is a directory:

```
tasks/{backlog|current|done}/{task-id}_{task-type}_{short-description}/
    description.md   # task body + frontmatter
    plan.md          # written by af.task.plan — the subject of this review
    review.md        # written later by af.task.review (post-implementation)
```

## Input

Single argument: the **task number** as a 4-digit string (e.g. `0001`, `0042`).

## Step 1 — Locate Task

Search `tasks/backlog/`, `tasks/current/`, `tasks/done/` for **directories** matching `{task-number}_*`.

| Found in         | Action                                             |
| ---------------- | -------------------------------------------------- |
| `tasks/done/`    | Tell user task already done. **Stop.**             |
| `tasks/backlog/` | Tell user to run `/plan-task {id}` first. **Stop.** |
| (none)           | Tell user task not found. **Stop.**                |
| `tasks/current/` | Continue to Step 2.                                |

Extract `{task-type}` and `{short-description}` from the directory name: `{task-number}_{task-type}_{short-description}`.

## Step 2 — Validate Handoff From /plan-task

Read `description.md` inside the task directory. Frontmatter must look like:

```yaml
---
id: 0006
type: feature
status: in-progress
topics: research, go
---
```

| Check                                    | On failure                                                                                                            |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Frontmatter present                      | Signal error to user, **stop**.                                                                                     |
| `id` equals `{task-number}`              | Signal error to user, **stop**.                                                                                     |
| `status` equals `in-progress`            | Signal specific error, **stop** (see below).                                                                        |
| `plan.md` exists in the task directory   | Tell user to run `/plan-task {id}` first. **stop**.                                                                 |

Status-specific messages:

- `pending` / `active` / `blocked` → _"Task {id} is `{status}`, not `in-progress`. Run `/plan-task {id}` and get the plan approved first."_
- `in-review` / `done` → _"Task {id} is `{status}` — past planning. Adversarial review runs before implementation."_

## Step 3 — Read the Plan and Its Intent

Read, in this order, before forming any challenge:

1. `plan.md` in full — this is the subject under review.
2. `description.md` in full, including `## Acceptance Criteria`, `## Out of scope`, `## Verification`, and any `## Clarification` Q&A.
3. Every doc the plan or description **already** links (ADRs, architecture chapters, guideline files).

Hold two facts side by side: what the task actually *requires* (from `description.md` acceptance criteria + out-of-scope), and what the plan *proposes to do*. The gap between them is where the holes live.

## Step 4 — Load the Current Architecture and Decisions

The plan is only "necessary" relative to what already exists. Read:

1. `docs/architecture/` — at minimum `05-building-block-view.md` (package layout + invariants) and `08-concepts.md`; read others (`04-solution-strategy.md`, `06-runtime-view.md`, `11-technical-risks.md`) when the plan touches their concern.
2. `docs/adr/` — every ADR whose subject overlaps the plan. Do not skim titles only; an ADR that already decided a question the plan re-opens is the highest-value hole to surface.
3. The "must read" guidelines regardless of topics: `docs/guidelines/clean_architecture.md`, `clean_code.md`, `domain_model.md`, `solid.md`, `testing.md`, plus `docs/guidelines/{topic}.md` for each `topics` entry.

`CLAUDE.md` at repo root also enumerates package responsibilities and critical invariants — treat it as an index into the above.

## Step 5 — Hunt for Holes (Adversarial Subagents)

Spawn adversarial subagents **in parallel** (single message, multiple Agent tool uses), one per lens. Each is told: *find reasons to cut or challenge the plan; a plan step is guilty until proven necessary.* Required lenses:

- **Necessity / YAGNI** — Which plan steps are not traceable to a specific `## Acceptance Criteria` item? Which introduce abstraction, configuration, or generality the task did not ask for? What could be deleted with no acceptance criterion failing?
- **Redundancy vs. existing code** — Does the plan re-implement a helper, pattern, or package responsibility that already exists (per `CLAUDE.md` package map, the building-block view, or the codebase)? Point at the `file:line` that already does it.
- **ADR / architecture contradiction** — Does any step violate a critical invariant (plan-before-apply, managed-surfaces fence, drift-vs-update, single-ownership, source-of-truth) or re-decide something an ADR already settled? Cite the ADR number.
- **Scope creep** — Does the plan do anything listed under `## Out of scope`, or anything not covered by an acceptance criterion or an explicit refactor allowance? A hunk that falls under out-of-scope is a contradiction, not an escape hatch.

Each subagent returns a flat list of findings; each finding is `{lens, plan-step-or-quote, why-suspect, cited-source file:line-or-ADR, proposed-cut-or-change}`. Instruct them: **return findings only, no edits**; prefer "cut" over "rework"; if a step is genuinely necessary, say nothing about it.

For small plans (a handful of steps) you may run the lenses inline instead of spawning subagents — the lenses matter, the mechanism does not.

## Step 6 — Consolidate Into a Question Set

Merge the findings:

- Deduplicate overlapping findings; keep the strongest framing and cite every relevant source.
- Drop findings that the plan or changelog already justifies, or that the description explicitly puts in scope.
- Rank by leverage: a step that contradicts an ADR or duplicates existing code outranks stylistic nits.

Turn each surviving finding into **one pointed, adversarial question** for the user. Frame every question so the default pull is toward removal or grounding, e.g.:

- _"Plan step 4 adds a `Foo` interface seam. No acceptance criterion needs more than one implementation. ADR 0021 already routes this through the strategy table. Cut the seam?"_
- _"Step 6 writes a new hashing helper, but `utils` already exposes one (`utils/hash.go:12`). Reuse it instead of adding a second?"_

If Steps 3–5 surface **no** holes, say so plainly and skip to Step 9 — do not manufacture questions to justify the review.

## Step 7 — Conduct the Adversarial Interview

Run the interview using the `grilling` skill, seeded with the question set from Step 6.

Rules (from `grilling`, reinforced here):

- **One question at a time.** Wait for the user's answer before asking the next. Batching questions is bewildering.
- For each question, state your **recommended answer** (almost always: cut it / ground it / defer it) and the source that motivates it.
- If a question can be resolved by reading the codebase or docs, resolve it yourself first and present the finding rather than asking blind.
- Accept the user's ruling. If they say a challenged step is necessary, it stays — record the justification; do not re-litigate.

Append every question + the user's resolution to `description.md` under the existing `## Clarification` section (create if absent), so the reasoning survives into implementation and later review:

```md
## Clarification

### Question

{adversarial question}

### Answer

{user's ruling — keep / cut / change, with the reason}
```

## Step 8 — Trim the Plan

Apply the interview outcomes to `plan.md` in place:

- **Cut** steps the user agreed are unnecessary. Remove them cleanly; do not leave commented-out residue.
- **Ground** steps the user kept but that lacked a source — add the citation to the `## Assumption grounding` table.
- **Re-point** steps that duplicated existing code so they reuse the existing abstraction.
- Leave everything the review confirmed as necessary exactly as it was.

Preserve the plan's required shape: the `## Assumption grounding` section stays non-empty (or explicitly `_None_`), and every boundary-crossing acceptance criterion still has a real-stack DoD checkbox. Do not let trimming break those invariants.

Then record the outcome as a terse `## Adversarial review` section at the end of `plan.md` — one bullet per challenge, each stating the ruling and its one-line reason:

```md
## Adversarial review

- **Cut** — `Foo` interface seam (step 4): no criterion needs >1 impl; ADR 0021 already routes via the strategy table.
- **Grounded** — hashing helper (step 6): now reuses `utils/hash.go:12` instead of a new one.
- **Kept** — per-agent projection loop (step 3): user confirmed each agent needs a distinct target.
```

Keep it to the challenges that were actually raised; do not restate the whole plan. If nothing was challenged, write `_No holes found; plan survived review unchanged._` under the heading — an unchanged plan that survived the grilling is a valid, good result.

## Step 9 — Conclusion

Summarize briefly:

- What was challenged and what the user ruled (kept / cut / changed) — a short bullet list, mirroring the `## Adversarial review` section now recorded in `plan.md`.
- Confirm `plan.md` now contains only necessary work, and its path.
- Confirm task status is unchanged (`in-progress`).
- **Next step:** run `/implement-task {task-number}` (ideally in a fresh session).

## Stopping Conditions Summary

| Condition                          | Response                                            |
| ---------------------------------- | --------------------------------------------------- |
| Task in `done/`                    | Inform user, stop                                   |
| Task in `backlog/`                 | Tell user to run `/plan-task {id}` first, stop      |
| Task not found                     | Inform user, stop                                   |
| Missing frontmatter                | Tell user to fix, stop                              |
| `status` not `in-progress`         | Signal specific error, stop                         |
| `plan.md` missing                  | Tell user to run `/plan-task {id}` first, stop      |
| No holes found                     | Report clean bill, skip interview, leave plan as-is |
| Interview complete                 | Trim plan, summarize, point to `/implement-task`    |
