# Definition-of-Done Gate Fixtures

Three checked-in mini task directories used by task 0034's `## Verification`.
Each fixture is a self-contained triple — `description.md`, `plan.md`, and
`diff.patch` — sized to exercise exactly one branch of `af.task.review`
Step 6.5:

| Fixture         | Exercises           | Expected outcome                                                                                                        |
| --------------- | ------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `passes/`       | happy path          | 6.5a passes, 6.5b table all `met`, 6.5c every hunk resolves — reviewer dispatches Step 7 subagents.                     |
| `missing-ac/`   | Step 6.5a fail-fast | 6.5a stops with `LegacyTask — task is missing the required '## Acceptance Criteria' section per the task-workflow contract.` |
| `scope-creep/`  | Step 6.5c fail      | 6.5a and 6.5b pass; 6.5c reports the unresolved hunk (`internal/log/verbose.go:1` — no criterion, no refactor bullet, no changelog mechanical-follow-up).             |

Fixtures are inspection artifacts — the reviewer reads `description.md` + the
supplied `diff.patch` exactly as if running Step 6 against `git diff`. They
never touch the real repo.
