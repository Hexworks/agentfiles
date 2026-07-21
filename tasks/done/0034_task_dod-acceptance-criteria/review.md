# Definition of Done + Acceptance Criteria review

The four-file diff for task 0034 lands the two things the description promised:
`af.create-task` now mandates the three-section body (`## Acceptance Criteria`,
`## Out of scope`, `## Verification`) and `af.task.review` gains a Step 6.5 DoD
gate that runs before the subagent burst. The gate itself passes for this
commit: every acceptance criterion is met from the diff, no unjustified hunks.

That said, the review found several structural weaknesses in the change:

- The producer/consumer contract between `af.create-task` and `af.task.review`
  is stated **by string**, in three places, and has already drifted in one
  sibling (`af.task.implement`) that this task deliberately did not touch. The
  drift is visible today (`status: active` still legal there; live task 0036
  uses it).
- The word "verifiable" — the load-bearing rule for the whole DoD idea — has
  three different definitions across Step 7, Step 8, and Step 6.5.
- Step 6.5 bundles a presence check, a per-criterion evidence check, and a
  scope-creep audit under one heading with one collapsed pass/fail row.
- The Step-7 template markdown code block is malformed (nested same-length
  fences); the rendered output silently disagrees with the source.
- No ADR, changelog, or glossary entry captures the new workflow contract; the
  example ACs mostly encode structural checks rather than behavior; task 0034's
  own `## Verification` is not reproducible.

The findings below are ordered roughly by blast radius. Pick one solution
checkbox per issue for `af.task.review-apply` to implement.

## Task-workflow contract fragmented across skills and already drifted

> [!WARNING]
>
> - [Clean Architecture — Common Closure Principle](../../../docs/guidelines/clean_architecture.md#common-closure-principle)
> - [Domain Model — Use The Project Language](../../../docs/guidelines/domain_model.md#use-the-project-language)
> - [SOLID — Dependency Inversion Principle](../../../docs/guidelines/solid.md#dependency-inversion-principle)

Three vocabularies now live in the `af.task.*` family and each is redeclared in
every skill that touches it: the **status enum** (`pending|in-progress|blocked|
in-review|done`), the **type enum** (`feature|bug|task|docs`), and the
**required body sections** (`## Acceptance Criteria`, `## Out of scope`,
`## Verification`). The commit tightens two of them in `af.create-task` but
leaves the sibling `af.task.implement/SKILL.md` untouched, and the drift is
already visible on disk:

```markdown
# af.create-task/SKILL.md — this commit

Step 6: "active is not a valid status — pending|in-progress|blocked|in-review|done"
Step 2: type options = feature|bug|task|docs
Step 7: template body must carry ## Acceptance Criteria / ## Out of scope / ## Verification

# af.task.implement/SKILL.md:60-61 — untouched, contradicts the above

`type` | One of `feature`, `bug`, `task`, `spike`
`status` | One of `pending`, `active`, `blocked`, `in-review`, `done`

# tasks/current/0036_feature_split-projects-out-of-profile/description.md:4

status: active
```

`af.task.review` Step 6.5 only reads `## Acceptance Criteria` — the other two
mandated sections are never checked by the consumer, so a task that renames
`## Out of scope` to `## Non-goals` silently passes the gate on the review
side and silently violates the create-side contract. There is no canonical
document either skill points at, so a future rename of any heading breaks the
pipeline invisibly.

Pick one:

- [ ] Extract a canonical `docs/guidelines/task_workflow_contract.md` (or a
      section in `tasks/README.md`) that lists the status enum, the type enum,
      and the three required body sections in exactly one place, and rewrite
      `af.create-task`, `af.task.implement`, and `af.task.review` to link to
      that doc instead of restating the values. Also fix
      `af.task.implement/SKILL.md:60-61` in this task's scope and migrate
      `tasks/current/0036_.../description.md` off `status: active`.
- [ ] Narrower fix: leave the contract distributed but stop the drift right now
      — update `af.task.implement/SKILL.md:60-61` to match the new enums, add
      `## Out of scope` and `## Verification` presence checks to
      `af.task.review` Step 6.5, migrate task 0036 off `status: active`, and
      add a one-line note in each SKILL header ("if you change this enum,
      update the other two skills too"). No new doc, but every skill still
      states the contract locally.
- [x] Promote the DoD sections to structured frontmatter (`acceptance_criteria:
  [...]`, `out_of_scope: [...]`, `verification: ...`) so the review gate
      reads named fields instead of grepping headings, and fix the enums in
      `af.task.implement` at the same time. Highest cost, strongest guarantee.

## `af.task.implement` bypasses the new required-sections contract

> [!WARNING]
>
> - [Clean Architecture — Stable Dependencies Principle](../../../docs/guidelines/clean_architecture.md#stable-dependencies-principle)
> - [Clean Code — Design Rules](../../../docs/guidelines/clean_code.md#design-rules)

The pipeline is `af.create-task` → `af.task.implement` → `af.task.review`. This
task adds enforcement at the two ends of the pipe but leaves the middle stage
silent: `af.task.implement` Step 2 validates frontmatter (`id`, `type`,
`status`, `topics`) but never checks that the required body sections exist or
that `## Acceptance Criteria` is non-empty. A task whose AC block was deleted
by hand runs happily through implement, then only fails at Step 6.5 of review
— maximally late, after the diff is already produced.

```markdown
# af.task.implement/SKILL.md Step 2 validates:

# id, type, status, topics

# It does not validate:

# presence of ## Acceptance Criteria / ## Out of scope / ## Verification

# non-emptiness of ## Acceptance Criteria
```

Pick one:

- [x] Add a `Step 2.5 — Body-sections gate` to `af.task.implement/SKILL.md`
      that mirrors `af.task.review` Step 6.5 rule 1: refuse to start if any of
      the three required sections is missing or `## Acceptance Criteria` is
      empty. Stop with the same "predates convention" message so behaviour is
      uniform across the pipeline.
- [ ] Keep implement unchanged but document explicitly in the task-workflow
      doc that the DoD contract is enforced only at creation and at review;
      implement trusts the input. Chosen when the extra check is judged too
      costly for what it catches.

## Definition of Done duplicated between Step 7 and Step 8, and missing from the glossary

> [!WARNING]
>
> - [Clean Code — Code Smells (needless repetition)](../../../docs/guidelines/clean_code.md#code-smells)
> - [Domain Model — Use The Project Language](../../../docs/guidelines/domain_model.md#use-the-project-language)

The rule "the AC checklist **is** the Definition of Done" is stated in Step 7
(lines 77-80) and half-restated in Step 8 (lines 121-122). Two authoritative
statements of the same rule is exactly the "duplicated rules that can drift
apart" smell — a future edit to Step 7 can silently miss Step 8. Meanwhile
neither `Definition of Done` nor `Acceptance Criteria` appears in
`docs/glossary.md`, so any third skill (`af.task.review-apply`, changelog
templates) has no canonical entry to link to.

```markdown
# Step 7 (lines 77-80)

The acceptance-criteria checklist **is** the Definition of Done: a task is done
when every box is `[x]` and `## Verification` passes.

# Step 8 (lines 121-122)

These two sections are the task's Definition of Done — `af.task.review` gates
on them, so a vague or empty checklist will block review later.
```

Pick one:

- [x] Keep the DoD definition in Step 7, replace the Step 8 restatement with a
      bare pointer ("these are the DoD sections defined in Step 7"), and add
      `Definition of Done` + `Acceptance Criteria` entries to
      `docs/glossary.md` so both skills point at the glossary.
- [ ] Move the DoD definition to a single "Definition of Done" callout at the
      top of `af.create-task/SKILL.md` and delete the two duplicate paragraphs;
      still add glossary entries.
- [ ] Add glossary entries only; leave the two in-skill statements as-is. Ships
      the smallest change but does not fix the drift risk.

## "Verifiable" is used as a hard gate but never defined once

> [!WARNING]
>
> - [Clean Code — Understandability](../../../docs/guidelines/clean_code.md#understandability)
> - [Testing — Test One Behavior At A Time](../../../docs/guidelines/testing.md#test-one-behavior-at-a-time)

The whole DoD gate hangs on the word "verifiable", and the diff uses it three
different ways:

```markdown
# af.create-task Step 7: "behavioral ones name a concrete `input → output` or a one-line smoke step"

# af.create-task Step 8: "if you cannot state how you'd check it, rewrite it until you can"

# af.task.review Step 6.5: "judge met / unmet from the diff, citing concrete evidence (`file:line`)"
```

None of these is a decision rule the reviewer or the create-side agent can
execute deterministically — two runs on the same input can legitimately
disagree. Under the current wording, a criterion like
`app.Service exposes all eight new methods with the exact signatures above`
(example-1) slips through as verifiable-by-grep, which the testing guidelines
explicitly warn against.

Pick one:

- [ ] Define `verifiable` in one place (glossary or the task-workflow doc):
      "either (a) a named test + expected assertion, (b) an observable
      input→output pair, or (c) a reproducible CLI/TUI smoke step with the
      expected result". Reference that definition from Step 7, Step 8, and
      Step 6.5.
- [x] Turn Step 6.5 rule 2 into a table the reviewer produces per criterion
      (`criterion | diff-evidence (file:line) | verdict | reason if unmet`),
      with an explicit `unverifiable` verdict when no evidence form applies.
      Definition still needs to live somewhere; put it inline in Step 6.5.

## Step 6.5 conflates three orthogonal checks under one gate

> [!WARNING]
>
> - [SOLID — Single Responsibility Principle](../../../docs/guidelines/solid.md#single-responsibility-principle)
> - [Clean Architecture — Common Reuse Principle](../../../docs/guidelines/clean_architecture.md#common-reuse-principle)
> - [Clean Code — Naming](../../../docs/guidelines/clean_code.md#naming)

Step 6.5 is titled "Definition-of-Done Gate" but runs three distinct checks:
(a) presence of `## Acceptance Criteria`, (b) per-criterion met/unmet
judgement against the diff, (c) scope-creep audit ("every diff hunk must
trace to a criterion"). Presence is a cheap syntactic guard; met/unmet is
intent→output diffing; scope-creep is the _inverse_ direction (hunk →
criterion). All three failure modes collapse to one row in the decision
table:

```markdown
| Result                                   | Action                         |
| ---------------------------------------- | ------------------------------ |
| All criteria met AND no scope creep      | Continue to Step 7.            |
| Any criterion unmet OR scope creep found | **STOP.** Report the gap list… |
```

That row does not distinguish "your description is wrong" from "your
implementation is wrong" from "your diff has extra work" — three different
fixes, one report shape. It also hides the second gate (scope-creep) from
anyone greping the doc for `scope`.

Pick one:

- [x] Split Step 6.5 into 6.5a "Contract presence" (fail fast, no diff read
      needed), 6.5b "DoD evidence" (per-criterion met/unmet), and 6.5c
      "Scope-creep audit" (hunk-to-criterion tracing). Each substep gets a
      single failure mode and a distinct user-facing report.
- [ ] Keep one section but rename it (e.g. "Step 6.5 — Intent Gate (DoD +
      scope-creep)") and expand the decision matrix so presence, DoD, and
      scope-creep have separate rows with separate remediation text.
- [ ] Keep the current shape but state explicitly in prose that Step 6.5 owns
      three concerns and Step 7 owns the rest; smallest change, weakest
      guarantee against future creep.

## Scope-creep rule has two escape hatches with no precedence

> [!WARNING]
>
> - [Clean Code — Understandability](../../../docs/guidelines/clean_code.md#understandability)
> - [Testing — Keep Tests Isolated](../../../docs/guidelines/testing.md#keep-tests-isolated)

Step 6.5 rule 3 offers two ways for a hunk to _not_ be scope creep:

```markdown
3. **Scope creep:** every diff hunk must trace to a criterion, or to a refactor
   that respects `## Out of scope`. A hunk that maps to no criterion and is not
   justified by the changelog is a finding.
```

Both escape hatches are underspecified:

- "Respects `## Out of scope`" conflates directions — `## Out of scope` is a
  _negative_ list ("things NOT being done"), so by construction a hunk that
  falls under it must **not** exist. A refactor mentioned in `## Out of scope`
  is contradictory with the section's meaning.
- "Justified by the changelog" is a moving target: the changelog is written by
  the same agent that produced the diff, so a self-justifying paragraph can
  neutralise the creep check. Two runs of the gate on the same input can
  legitimately disagree.

Pick one:

- [x] Rewrite the rule as an ordered check: `hunk maps to (a) a criterion,
  else (b) an explicit "refactor allowed" bullet inside description.md,
  else (c) a changelog entry that names the hunk as a mechanical
  follow-up (fmt, import order, generated file). Otherwise → finding.`
      Drop the `## Out of scope` escape clause entirely.
- [ ] Keep both escape hatches but tighten the definitions in prose: "Out of
      scope" clause fires only when the hunk _removes_ something the section
      names; "justified by changelog" fires only for the whitelist above.

## Step 8 grilling gate drops `## Verification` — three-section contract not enforced end-to-end

> [!WARNING]
>
> - [SOLID — Liskov Substitution Principle](../../../docs/guidelines/solid.md#liskov-substitution-principle)

Step 7 mandates **three** sections. Step 8's grilling exit gate enforces only
**two**:

```markdown
The interview **must not finish** until: - `## Acceptance Criteria` has **≥1** checkbox, every criterion verifiable
(if you cannot state how you'd check it, rewrite it until you can), and - `## Out of scope` is filled (`- none` is allowed only when nothing is
genuinely excluded).
```

So a task can legally exit grilling with `## Verification` empty, and — because
Step 6.5 only reads `## Acceptance Criteria` — the review gate never notices.
Contract stated as "three sections", enforced as "two".

Pick one:

- [x] Add `## Verification` to the Step 8 grilling exit condition (non-empty
      command block), and add a matching presence check to Step 6.5 of
      `af.task.review`.
- [ ] Keep Step 8 as two-of-three, but state explicitly in Step 7 that
      `## Verification` is authored later (e.g. during implement) and remove
      the "three required sections" phrasing.

## "Predates convention" is un-named, and the review skill couples to its sibling by name

> [!WARNING]
>
> - [Domain Model — Model Constraints Explicitly](../../../docs/guidelines/domain_model.md#model-constraints-explicitly)
> - [Clean Architecture — Acyclic Dependencies Principle](../../../docs/guidelines/clean_architecture.md#acyclic-dependencies-principle)

The gate introduces a genuinely new **domain outcome** — "in-review task, no
`## Acceptance Criteria`, so review cannot run" — but names it in prose as
"the task predates the acceptance-criteria convention", and does so with a
direct reference to the sibling skill:

```markdown
# af.task.review Step 6.5

Missing or empty → **STOP**. Tell the user the task predates the
acceptance-criteria convention (see `af.create-task`) and must add a
verifiable `## Acceptance Criteria` checklist before review can run.

# af.task.review Stopping-Conditions

| `## Acceptance Criteria` missing/empty | Tell user task predates convention, stop before dispatch |
```

Two consequences: (1) the phrasing already varies between the two occurrences
inside the same file, so it will drift further; (2) the review's error text
depends on the identity of the producer skill, which breaks the day a task is
authored by hand or by a new import skill.

Pick one:

- [x] Give the outcome a single name (e.g. `LegacyTask` / `PreConventionTask`) + a glossary entry, rephrase the stop message to reference the _contract_
      ("task is missing the required `## Acceptance Criteria` section — see
      the task-workflow contract doc"), and use the same phrase in the
      Stopping-Conditions row.
- [ ] Keep "predates convention" phrasing but delete the `(see af.create-task)`
      producer reference and make the two occurrences match verbatim. Add a
      short paragraph explaining whether legacy tasks should be back-filled or
      reviewed under a legacy path.

## `## Verification` template is a boilerplate build-and-test, not per-task behavior

> [!WARNING]
>
> - [Testing — Start With The Smallest Useful Test](../../../docs/guidelines/testing.md#start-with-the-smallest-useful-test)
> - [Testing — Test One Behavior At A Time](../../../docs/guidelines/testing.md#test-one-behavior-at-a-time)

The Step-7 template hard-codes `make build && make test && make lint` as the
primary Verification line and treats the smoke line as an optional comment:

```markdown
## Verification

`​`​`
make build && make test && make lint

# + any manual smoke line, e.g. ./bin/af → <screen> → <action>

`​`​`
```

Because the AC checklist **is** the DoD and DoD passes when Verification
passes, a green `make test` marks the task done even when no test covers the
new behavior. That is precisely the anti-pattern the testing guidelines warn
against ("broad workflow tests when a unit test can prove the same rule").

Pick one:

- [x] Reword the template so `make build && make test && make lint` is a
      baseline gate and the section **requires** at least one behavior-specific
      line (test name, `go test -run TestX`, or reproducible smoke input→output)
      before Step 8 counts it as filled.
- [ ] Weaker fix: leave the template alone but require the smoke line to be
      uncommented (drop the `# + …` optional framing) so authors always add a
      task-specific line.

## Example ACs mostly restate implementation, not observable behavior

> [!WARNING]
>
> - [Testing — Assert Behavior, Not Mock Mechanics](../../../docs/guidelines/testing.md#assert-behavior-not-mock-mechanics)
> - [Domain Model — Model Constraints Explicitly](../../../docs/guidelines/domain_model.md#model-constraints-explicitly)

The two example descriptions are the canonical model authors will copy. Their
shape sets the ceiling for AC quality across the repo, and today they mix
structural checks with a `make build && make test && make lint pass`
end-marker AC that duplicates the Verification block:

```markdown
# example-1

- [ ] `app.Service` exposes all eight new methods with the exact signatures above.
- [ ] Each method has a unit test for happy path + ≥1 error path.
- [ ] `make build && make test && make lint` pass.

# example-2

- [ ] `make build && make test && make lint` pass; manual smoke per Verification.
```

`exposes eight methods` is a grep-check, not a behavior; `Each method has a
unit test for happy path + ≥1 error path` is a coding standard, not a
task-specific rule; `make … pass` repeats `## Verification` and teaches
authors that "green build = criterion".

Pick one:

- [x] Rewrite the structural ACs in both examples into behavioural pairs (e.g.
      `Service.LoadProfiles returns joined errors when 2 of 3 profile dirs are
  malformed; test TestLoadProfilesJoinsPerProfileErrors`), drop the meta
      ACs (`Each method has a unit test…`), and delete the trailing
      `make build && make test && make lint pass` bullet from both examples.
- [ ] Keep the current examples but add a short "AC style" note in
      `af.create-task` Step 7 that says: prefer `given X, doing Y produces Z`
      over `component exposes method M`; and drop only the duplicate `make …
  pass` bullet from both examples.

## Task 0034's own `## Verification` is not reproducible

> [!WARNING]
>
> - [Testing — Keep Tests Isolated](../../../docs/guidelines/testing.md#keep-tests-isolated)
> - [Documentation — Document Current Reality First](../../../docs/guidelines/documentation.md#document-current-reality-first)

The verification section of this task lists three process-level cases —
`af.create-task` dry-run, review happy path, review fail path — but with no
input fixtures and no expected-output strings:

```markdown
- Dry-run `af.create-task`: generated `description.md` has the 3 sections + valid status.
- `af.task.review` happy path: AC met → Step 6.5 passes → subagents dispatch.
- `af.task.review` fail path: unmet AC or stray hunk → stops at 6.5, no dispatch.
```

If the skill is ever edited again, there is no way to rerun the same
verification — a future reviewer must invent fresh inputs. This is the
"shared state that can change another test's result" failure mode applied to
process tests.

Pick one:

- [x] Add three checked-in fixture task directories under
      `.claude/skills/af.task.review/fixtures/` (passes, missing-AC,
      scope-creep) and rewrite the Verification bullets as: "run
      `/review-task 0000-fixture-A` → expect dispatch; run
      `/review-task 0000-fixture-B` → expect stop message `X`". Also spell
      out the expected stop-message strings so a reviewer can grep the
      transcript.
- [ ] Accept the inspection-based verification as-is for this doc task and
      add a note under `## Verification` acknowledging it (documents current
      reality without adding fixtures).

## Nested triple-backtick fence in Step-7 template breaks Markdown rendering

> [!WARNING]
>
> - [Clean Code — Code Smells (opacity)](../../../docs/guidelines/clean_code.md#code-smells)

The Step-7 template wraps a `markdown` code block and, inside it, opens a
second same-length code block around the shell snippet:

````
.claude/skills/af.create-task/SKILL.md:82   ```markdown
                                            ---
                                            id: NNNN
                                            ...
:104                                        ```
                                            make build && make test && make lint
:107                                        ```
:108                                        ```
````

Under CommonMark the inner opening fence on line 104 closes the outer fence
(identical length, no info string), so the outer `markdown` block terminates
early. The renderer produces broken output; the source and the rendered
template disagree.

Pick one:

- [x] Change the outer fence to four backticks (` ```` markdown … ```` `) so
      the inner three-backtick fence is preserved. Minimal-diff fix.
- [ ] Use an indented (4-space) code block for the inner shell snippet inside
      the outer `markdown` fence. Slightly more disruptive but avoids nested
      fences entirely.

## No ADR, changelog, or arc42 note for a durable workflow-contract change

> [!WARNING]
>
> - [Documentation — Update The Right Artifact](../../../docs/guidelines/documentation.md#update-the-right-artifact)
> - [Clean Architecture — Architecture Boundaries](../../../docs/guidelines/clean_architecture.md#architecture-boundaries)

Introducing three required body sections and a mandatory review gate is a
durable change to the _workflow architecture_ — precisely the kind of decision
ADRs exist to record. The commit modifies four SKILL files but leaves
`docs/adr/`, `docs/architecture/`, and `docs/changelog/` untouched. The
`af.task.review` skill itself says (Step 3): _"If the plan or changelog is
missing, note that as a finding for the review."_

```
# git show --stat ecae862
 .claude/skills/af.create-task/SKILL.md                 | 47 +++++++++++++++++++---
 .claude/skills/af.create-task/example-1-description.md | 12 ++++++
 .claude/skills/af.create-task/example-2-description.md | 14 +++++++
 .claude/skills/af.task.review/SKILL.md                 | 25 ++++++++++++
# no docs/adr/, docs/architecture/, docs/changelog/ files touched
```

Precedent for recording similar decisions exists (ADR 0006 "TUI-only", ADR
0007 "typed errors").

Pick one:

- [x] Add three artifacts in this fix-up: (1) an ADR under `docs/adr/`
      recording the DoD-gate decision and the alternatives considered (EARS /
      spec-kit / 8th subagent), (2) a changelog entry
      `docs/changelog/2026-07-05_0034-dod-acceptance-criteria.md`
      summarising the four-file change, (3) a short note in the arc42
      building-block view (or the runtime view) that create/implement/review
      share a description-body contract.
- [ ] Add only the changelog entry (satisfies the skill's own Step 3
      requirement) and skip the ADR + arc42 update.
- [ ] Accept the omission; treat the SKILL files as self-documenting and add
      nothing.
