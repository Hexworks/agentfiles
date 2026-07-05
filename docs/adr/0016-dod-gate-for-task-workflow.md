# Definition-of-Done Gate For The Task Workflow

## Status

accepted

## Context

The `agentfiles` task workflow is a three-stage pipeline of Claude Code
skills — `af.create-task` → `af.task.implement` → `af.task.review` — that
takes a task from an initial idea through to a review-ready diff. Before
this decision the pipeline had no explicit intent-to-output contract:

- `af.create-task` produced a `description.md` whose body was free-form.
- `af.task.implement` validated frontmatter (`id`, `type`, `status`,
  `topics`) but never checked the body.
- `af.task.review` dispatched seven parallel review subagents
  (security / clean-code / clean-architecture / SOLID / DDD / testing /
  Go) against the diff, independent of what the task actually asked for.

The result was recurring `drift:` commits — gaps between what the author
wanted and what the workflow produced — visible in the repo's git log.
Two root causes:

1. `description.md` carried no required Definition of Done, so the
   implementing agent stopped at "looks done".
2. `af.task.review` checked craftsmanship (7 subagent axes) but never
   checked that *what was asked got built and nothing extra crept in*.

Fix candidates considered:

- **EARS-style requirements syntax**, Spec-Kit multi-file specs, or
  Given-When-Then acceptance tests. Rejected as too verbose and rigid for
  a workflow that already prizes token economy; see the recorded memory
  `feedback_token_economy.md`.
- **An 8th review subagent** dedicated to intent checking. Rejected: it
  doubles review latency and cost for a check that reduces to reading two
  files (`description.md` + `git diff`) and does not benefit from parallel
  dispatch.
- **Structured frontmatter fields** (`acceptance_criteria: [...]`,
  `out_of_scope: [...]`, `verification: ...`) so the review gate reads
  named fields instead of grepping headings. Considered during the
  fix-up for this decision (task 0034 review, issue 1); rejected in
  favor of keeping the Definition of Done in the description body,
  where a human reader can scan it without a YAML parser.

## Decision

The workflow gains an explicit **Definition of Done contract** and a
gate that enforces it before the expensive review-subagent dispatch.

Contract (three required sections in every `description.md` body):

- `## Acceptance Criteria` — hybrid checklist. Each item is verifiable —
  either a named test + expected assertion, an observable input→output
  pair, or a reproducible CLI/TUI smoke step. The checklist **is** the
  Definition of Done for the task; there is no separate DoD section.
- `## Out of scope` — negative list. `- none` allowed only when nothing
  is genuinely excluded.
- `## Verification` — bullet list. First bullet is the baseline gate
  (`make build && make test && make lint`); at least one further bullet
  names behavior-specific evidence.

Enforcement points:

- `af.create-task` Step 7 writes the three sections into every new
  `description.md`. Step 8's grilling interview cannot finish until all
  three are non-empty and Acceptance Criteria carries at least one
  verifiable checkbox.
- `af.task.implement` Step 2.5 refuses to enter plan mode if any of the
  three required sections is missing or Acceptance Criteria is empty.
  Signals `LegacyTask`.
- `af.task.review` gains **Step 6.5**, a mandatory Definition-of-Done
  Gate split into three substeps:
    - **6.5a — Contract presence** (fail-fast, no diff read): the three
      required sections must be present and non-empty. Missing →
      `LegacyTask` stop, no diff read, no subagent dispatch.
    - **6.5b — DoD evidence** (per-criterion table): for each criterion
      the reviewer produces a row `criterion | diff-evidence (file:line)
      | verdict | reason`, with verdicts `met` / `unmet` / `unverifiable`.
      Any `unmet` or `unverifiable` verdict stops the gate.
    - **6.5c — Scope-creep audit** (ordered check): every diff hunk must
      resolve via, in order, (1) a criterion from 6.5b, (2) an explicit
      `- Refactor allowed: <scope>` bullet in the description, or (3) a
      changelog entry naming the hunk as a mechanical follow-up (fmt,
      import order, generated file, rename-only). Otherwise the hunk is
      an unresolved-scope-creep finding.

Step 6.5 runs before Step 7 so a failed gate short-circuits the expensive
subagent burst.

Canonical vocabulary for the contract lives in `docs/glossary.md`
(`Acceptance Criteria`, `Definition of Done`, `Legacy Task`), so the
three skills can point at the glossary instead of restating the rules.

## Consequences

Positive:

- The pipeline can no longer silently produce a task whose diff does not
  answer the question the task asked. Every diff is judged against a
  verifiable checklist and a scope-creep audit before craftsmanship
  review begins.
- Failed gates cost only the orchestrator's tokens — the seven Step-7
  subagents never fire for a `LegacyTask`, an unmet criterion, or a
  scope-creep hunk.
- The DoD vocabulary is now first-class in the glossary, so future
  skills (a changelog-review skill, an import skill, an alternative
  reviewer) share the same terms.

Negative / accepted:

- Existing `in-review` tasks whose descriptions predate the contract
  now signal `LegacyTask` and must be back-filled before review can run.
  This is deliberate: the review already flagged such tasks as findings;
  the gate promotes the finding into an explicit stop.
- `af.create-task` Step 8's grilling interview grew a third exit
  condition (`## Verification` non-empty), which slightly lengthens the
  interview. This is the price of enforcing the contract at the
  producer side.
- Authors must supply behavior-specific evidence in `## Verification`;
  the previous "green `make …`" ending is no longer sufficient. This
  is intentional (see the testing guidelines) and follows the same
  spirit as the "assert behavior, not mock mechanics" rule.

## Notes

Enforcement uses no new Go source: everything lives in the four SKILL
files (`af.create-task`, `af.task.implement`, `af.task.review`, and
`af.task.review-apply`), plus three fixture task directories under
`.claude/skills/af.task.review/fixtures/` (`passes/`, `missing-ac/`,
`scope-creep/`) that exercise each 6.5 substep for regression.
