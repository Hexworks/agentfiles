# 0034 changes

Task 0034 introduces a Definition-of-Done contract into the `agentfiles`
task workflow so the `af.create-task` → `af.task.implement` →
`af.task.review` pipeline can no longer produce a diff that fails to
answer the question the task asked. The initial commit (`ecae862`)
landed the two headline pieces — `af.create-task` mandating a three-section
body and `af.task.review` gaining a Step 6.5 gate — and this fix-up
completes the contract across the pipeline: the sibling `af.task.implement`
skill is brought back into line, the gate is split into three substeps
with distinct failure modes, the vocabulary lands in the glossary, the
decision is recorded as ADR 0016, and three checked-in fixtures make the
gate reproducible.

## Decisions

- Keep the Definition of Done in the description body (three markdown
  sections) rather than promoting it to structured frontmatter — **Why:**
  a human reader can scan a body section without a YAML parser, and the
  three-section shape is already the mental model authors carry from
  the example descriptions.
- Split Step 6.5 into three substeps (6.5a presence, 6.5b DoD evidence,
  6.5c scope-creep) with distinct failure modes and distinct user-facing
  reports — **Why:** a single collapsed gate row hid three orthogonal
  failure modes (author's description wrong / implementation wrong /
  diff has extra work) behind one message.
- Name the "task predates the acceptance-criteria convention" outcome
  `LegacyTask` and give it a glossary entry — **Why:** the earlier prose
  varied between the two occurrences that mentioned it, and it coupled
  the review skill to its sibling by name; a domain term decouples the
  outcome from the producer skill.
- Drop the `## Out of scope` escape clause from the scope-creep audit —
  **Why:** `## Out of scope` is a negative list, so a hunk that falls
  under it is by construction a contradiction, not an allowance.

## Assumptions

- Task 0036 has not been picked up by `af.task.implement` (no `plan.md`
  in its directory), so migrating its `status: active` → `status: pending`
  matches the expected pre-implementation state — **Why:** flipping to
  `in-progress` would misrepresent the current state; `pending` is
  what `af.create-task` would have written under the fixed enums.

## Other Notes

- ADR 0016 records the DoD-gate decision, the alternatives considered
  (EARS, spec-kit multi-file specs, an 8th subagent, structured
  frontmatter), and the consequences.
- Glossary entries added: `Acceptance Criteria`, `Definition of Done`,
  `Legacy Task`.
- arc42 building-block view (`docs/architecture/05-building-block-view.md`)
  gained a short note about the task-workflow skill contract shared by
  `af.create-task`, `af.task.implement`, and `af.task.review`.
- Three fixtures under `.claude/skills/af.task.review/fixtures/`
  (`passes/`, `missing-ac/`, `scope-creep/`) each contain a triple
  `description.md` + `plan.md` + `diff.patch` sized to exercise exactly
  one 6.5 branch. Task 0034's own `## Verification` bullets now cite
  them by fixture name plus the expected stop message.

## af.task.implement enums brought into line

The sibling skill still allowed `spike` as a task type and `active` as a
status — values the other two skills of the pipeline had already
outlawed. Step 2.5 gate mirrors `af.task.review` Step 6.5a so the
pipeline enforces the contract at every stage.

```md
# before — .claude/skills/af.task.implement/SKILL.md
| `type`   | One of `feature`, `bug`, `task`, `spike` AND …
| `status` | One of `pending`, `active`, `blocked`, `in-review`, `done`
```

```md
# after
| `type`   | One of `feature`, `bug`, `task`, `docs` AND …
| `status` | One of `pending`, `in-progress`, `blocked`, `in-review`, `done`
```

Plus a new Step 2.5 refusing to enter plan mode when any of the three
required sections is missing or `## Acceptance Criteria` is empty —
same `LegacyTask` outcome as `af.task.review` Step 6.5a.

## Step 6.5 split into 6.5a / 6.5b / 6.5c

```md
# before
## Step 6.5 — Definition-of-Done Gate (Mandatory, runs BEFORE any subagent)

1. Read `## Acceptance Criteria` … Missing or empty → STOP.
2. For each criterion, judge met/unmet from the diff citing file:line.
3. Scope creep: every hunk must trace to a criterion or a refactor that
   respects `## Out of scope`. Otherwise finding.
4. Decision: all met AND no creep → continue; else STOP.
```

```md
# after — three substeps with distinct failure modes
### Step 6.5a — Contract presence (fail-fast, no diff read needed)
### Step 6.5b — DoD evidence (per-criterion table with met/unmet/unverifiable verdicts)
### Step 6.5c — Scope-creep audit (ordered check: criterion, refactor-allowed bullet, changelog mechanical follow-up)
```

The Stopping-Conditions summary table gained one row per substep so the
error mode is visible at a glance.

## Verification template requires behavior-specific evidence

`## Verification` is now a bullet list, not a shell block. The first
bullet is the baseline (`make build && make test && make lint`); at
least one further bullet must name behavior-specific evidence — a
named test, a `go test -run` invocation, or a reproducible smoke
input→output. A `## Verification` with only the baseline bullet does
not count as filled at Step 8.

The Step-7 markdown template also gained a four-backtick outer fence so
the inner three-backtick shell snippet renders correctly under
CommonMark.

## Example ACs rewritten as behavioral pairs

Both example descriptions (`example-1`, `example-2`) previously mixed
structural checks (`app.Service exposes eight methods`), meta ACs
(`Each method has a unit test…`), and a duplicated `make … pass` bullet.
The rewrite pairs every AC with a concrete input→output plus a named
test (e.g. `TestLoadProfilesJoinsPerProfileErrors`,
`TestPlanProjectActionButtonMatrix`), drops the meta ACs, and moves the
baseline `make …` line to `## Verification` where it belongs.

## Glossary entries

Three new entries in `docs/glossary.md`: `Acceptance Criteria`,
`Definition of Done`, and `Legacy Task`. The review skill's stop
message now points at the contract (and the glossary term) instead of
naming the producer skill, so the message stays correct when a task is
authored by hand or by a future import skill.

## Task 0036 status migration

`tasks/current/0036_feature_split-projects-out-of-profile/description.md`
carried `status: active` — invalid under the enum documented in every
skill of the pipeline. Task has no `plan.md`, so `af.task.implement` has
not started; migrated to `status: pending`, matching what
`af.create-task` would have written today.
