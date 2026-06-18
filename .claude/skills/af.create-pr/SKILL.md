---
name: Create PR [Agentfiles]
description: Create a GitHub pull request for the current branch with a Jira ticket link, change summary, and the project's standard checklist. Use when the user asks to open/create a PR for this repository.
---

# Create Pull Request

## Overview

Creates a GitHub pull request for the current branch using `gh pr create`. The PR body always follows the template in [Step 7](#step-7-create-the-pr) — ticket link, description, checklist (with appropriate boxes ticked), and a notes section.

**Quality gate**: Before the PR is opened, this skill **MUST** run the local quality checks in [Step 2](#step-2-quality-gate-must-pass-before-proceeding) and refuse to proceed if any of them fail. This is non-negotiable — a PR is never created on top of a red local state.

This skill is intended to work across any project that has build, test, and lint/format steps. The ecosystem-specific commands are detected at runtime (see [Step 2](#step-2-quality-gate-must-pass-before-proceeding)) — there is no hard-coded toolchain.

## Algorithm

### Step 1: Gather branch state

Run these in parallel via the Bash tool:

- `git status` — confirm the working tree is clean (warn the user if not; do not create a PR with uncommitted changes unless they confirm).
- `git rev-parse --abbrev-ref HEAD` — current branch name.
- `git log origin/main..HEAD --pretty=format:'%h %s'` — all commits on the branch since it diverged from `main`. Prefer `origin/main` over local `main` (which may be stale); fall back to `git merge-base` discovery if needed. If the project's default branch is not `main` (e.g. `master`, `develop`, `trunk`), use that instead — detect with `git symbolic-ref refs/remotes/origin/HEAD`.
- `git diff origin/main...HEAD --stat` — high-level summary of changed files.
- `git diff origin/main...HEAD` — full diff, to inform the PR description and decide which checklist boxes to tick.
- `gh pr view --json number 2>/dev/null` — check a PR doesn't already exist for this branch.

### Step 2: Quality gate (MUST pass before proceeding)

**This gate is mandatory.** If any check below fails or produces new warnings, **stop** and report the failure to the user. Do not draft, push, or create the PR until they are green. The user can fix the issues and re-invoke the skill, or explicitly ask you to skip the gate — never skip it on your own initiative.

The gate validates four checklist items at once:

- _My changes generate no new warnings_
- _I have added/updated tests where necessary_
- _New and existing tests pass locally_
- _Static analysis / linters pass_

#### 2a. Detect the project's toolchain

Inspect the repo root (and `CLAUDE.md`/`AGENTS.md`/`README.md` if present) to identify the build, test, lint, and format commands. Common manifests and their typical commands:

| Manifest / marker                 | Ecosystem      | Build                                  | Test             | Lint                                         | Format check                                |
| --------------------------------- | -------------- | -------------------------------------- | ---------------- | -------------------------------------------- | ------------------------------------------- |
| `pyproject.toml` (with `uv.lock`) | Python (uv)    | `uv build`                             | `uv run pytest`  | `uv run ruff check .`                        | `uv run ruff format --check .`              |
| `pyproject.toml` (poetry/pip)     | Python         | `poetry build` / `python -m build`     | `pytest`         | `ruff check .` / `flake8`                    | `ruff format --check .` / `black --check .` |
| `package.json`                    | Node.js        | `npm run build` (if defined)           | `npm test`       | `npm run lint`                               | `npm run format:check`                      |
| `pnpm-lock.yaml`                  | Node.js (pnpm) | `pnpm build`                           | `pnpm test`      | `pnpm lint`                                  | `pnpm format:check`                         |
| `build.gradle(.kts)`              | JVM (Gradle)   | `./gradlew build` (already runs tests) | `./gradlew test` | `./gradlew check` / `ktlintCheck` / `detekt` | `./gradlew spotlessCheck` / `ktlintCheck`   |
| `pom.xml`                         | JVM (Maven)    | `mvn verify` (runs tests)              | `mvn test`       | `mvn checkstyle:check` / `spotbugs:check`    | `mvn spotless:check`                        |
| `go.mod`                          | Go             | `go build ./...`                       | `go test ./...`  | `golangci-lint run`                          | `gofmt -l .` (must be empty)                |
| `Cargo.toml`                      | Rust           | `cargo build`                          | `cargo test`     | `cargo clippy --all-targets -- -D warnings`  | `cargo fmt --check`                         |
| `mix.exs`                         | Elixir         | `mix compile --warnings-as-errors`     | `mix test`       | `mix credo --strict`                         | `mix format --check-formatted`              |

If `CLAUDE.md`/`AGENTS.md` documents the canonical commands, **prefer those** over the table — the project may wrap them with custom scripts (`make test`, `nx run-many`, `bazel test //...`, etc.).

If you cannot confidently identify the commands, **ask the user** which to run rather than guessing.

#### 2b. Run the gate commands in parallel

Run the detected test, lint, and format-check commands in parallel via the Bash tool. Each must:

- Exit 0.
- Produce no new warnings. For test runners that print a warnings summary (pytest, etc.), the summary must be empty. For compilers that emit warnings (rustc, javac, kotlinc, go vet, tsc), no new warnings on changed code.
- Format checks must report zero files needing reformatting.

If the build itself runs tests (e.g. `./gradlew build`, `mvn verify`, `cargo test` after `cargo build`), running the build is sufficient to cover both the test and "no warnings" items — note that and skip the redundant test invocation.

#### 2c. Tests-added-where-needed check

Decide from the diff gathered in Step 1: if the diff touches production source directories (`src/`, `lib/`, `app/`, `internal/`, `pkg/`, `cmd/`, etc.) but no test files changed (`test/`, `tests/`, `*_test.go`, `*Test.kt`, `*.test.ts`, `__tests__/`, etc.), ask the user to confirm tests aren't needed (e.g. pure refactor, generated-code update, docs-only change inside source). If the change clearly warrants tests and none were added, stop and tell the user — don't open a PR that will fail review.

#### 2d. External dependencies

If integration tests need external services (Docker daemon for testcontainers, a running database, etc.) and they aren't available, the gate run will error out — that still counts as a gate failure. Tell the user what's missing and re-run, rather than skipping the tests.

**Only after all four items pass** may you proceed to Step 3. Record the fact that the gate passed so you can tick the corresponding checkboxes in Step 5.

### Step 3: Extract the Jira ticket ID

- The ticket ID pattern is `[A-Z]+-\d+` (e.g. `SOC-277`) and appears **at the end** of commit subject lines, commonly wrapped in parentheses: `feat: foo (SOC-277)` or bare `fix: bar SOC-277`.
- Also check the branch name (e.g. `fix/safe-kafka-commits` may or may not contain it; branches like `feature/SOC-277-...` do).
- If **multiple different** ticket IDs appear across commits, ask the user which one the PR should link to (or whether to link multiple).
- If **no** ticket ID can be found, ask the user for it before proceeding.

Build the link as: `https://imtf.atlassian.net/browse/SOC-277` (replace the host with the project's Jira instance if different — check `CLAUDE.md` or the repo's existing PRs for the convention).

### Step 4: Draft title and description

- **Title**: short (≤ 70 chars), Conventional-Commits-style prefix if appropriate (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`), followed by a concise summary, and **the ticket ID appended in parentheses at the end**. Examples:
    - `fix: implement safe Kafka offset commits (SOC-277)`
    - `feat: add retry backoff to ACM consumer (SOC-301)`

    If the branch only has one commit and its subject already matches this shape, reuse it verbatim.

- **Description**: 2–5 sentences explaining _what_ changed and _why_. Focus on rationale and user-visible impact, not a file-by-file enumeration — the diff shows the what. Pull reasoning from commit bodies where available. This goes under `## 📝 Description`, replacing the `<!-- What does this PR do and why? -->` placeholder.

### Step 5: Decide which checklist boxes to tick

Tick only boxes you can **justify from the diff or the conversation**. Leave a box unchecked if you cannot verify it from here — do not tick things like "CI passes" or "Docker image builds" on faith.

Use this as a guide:

**Tickable from the diff alone (tick when the condition holds):**

- ✅ _My code follows the project's style guidelines_ — tick unless you knowingly diverged from conventions in this repo.
- ✅ _I have performed a self-review of my code_ — tick: you read the full diff in Step 1.
- ✅ _I have commented my code, particularly in complex areas_ — tick if non-trivial logic has explanatory comments, **or** if the change is simple enough that no comments were needed.
- ✅ _I have updated relevant documentation_ — tick if docs were updated (`docs/`, `AGENTS.md`, `CLAUDE.md`, `README.md`, inline docstrings/KDoc/JavaDoc, OpenAPI/AsyncAPI specs), **or** mark as checked only when no doc updates were warranted (skip ticking if you suspect docs _should_ have been updated but weren't).
- ✅ _Public APIs are properly documented_ — tick if public APIs changed and have docstrings/OpenAPI updates; also tick if no public APIs changed (N/A case).
- ✅ _Log messages follow project logging guidelines_ / _No sensitive information is logged_ — tick if logging wasn't touched **or** if new log calls follow the project's logging conventions and don't leak PII/secrets.
- ✅ _No unnecessary changes to Dockerfile or image size_ — tick if `Dockerfile`/docker-related files weren't changed, or the change is intentional and minimal.

**Tick only because the Step 2 quality gate passed (these four MUST be ticked — if any is not green, you should have stopped at Step 2 and never reached here):**

- ✅ _My changes generate no new warnings_ — the gate confirmed an empty warnings summary.
- ✅ _I have added/updated tests where necessary_ — the gate confirmed tests exist / aren't warranted (see Step 2c).
- ✅ _New and existing tests pass locally_ — the test command (or build that includes tests) passed.
- ✅ _Static analysis / linters pass_ — lint and format-check commands passed.

**Leave unchecked unless explicitly verified in-session:**

- ⬜ _Project builds successfully locally_ — needs a real build (tick only if a standalone build command was executed in addition to or as part of the gate).
- ⬜ _CI pipeline passes_ — needs CI to have run.
- ⬜ _Docker image builds successfully_ / _Application runs successfully in Docker_ — needs a real docker build/run.

If the user ran the build/Docker in this session and it succeeded, you may tick those boxes.

### Step 6: Push the branch if needed

- `git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null` — check for upstream.
- If no upstream, `git push -u origin HEAD`.
- If there's an upstream but local is ahead, `git push`.

### Step 7: Create the PR

Use `gh pr create` with a HEREDOC for the body to preserve formatting exactly. Fill in the `<TICKET-ID>`, the description, and the checkboxes (`- [x]` for ticked, `- [ ]` for unticked — decided in Step 4). Keep the horizontal rules (`---`), emoji headings, and section order **exactly** as shown — the template is checked into the repo and tooling/reviewers rely on it.

```bash
gh pr create --title "<title> (<TICKET-ID>)" --body "$(cat <<'EOF'
## 🎫 Ticket

## [<TICKET-ID>](https://imtf.atlassian.net/browse/<TICKET-ID>)

## 📝 Description

<2–5 sentence description of what changed and why>

---

## ✅ Checklist

### Code & Documentation

- [ ] My code follows the project's style guidelines
- [ ] I have performed a self-review of my code
- [ ] I have commented my code, particularly in complex areas
- [ ] I have updated relevant documentation
- [ ] Public APIs are properly documented

### Quality & Testing

- [ ] My changes generate no new warnings
- [ ] I have added/updated tests where necessary
- [ ] New and existing tests pass locally
- [ ] Static analysis / linters pass

### Logging

- [ ] Log messages follow project logging guidelines
- [ ] No sensitive information is logged

### Build & CI

- [ ] Project builds successfully locally
- [ ] CI pipeline passes

### Docker

- [ ] Docker image builds successfully
- [ ] Application runs successfully in Docker
- [ ] No unnecessary changes to Dockerfile or image size

---

## 💡 Notes

<Additional context, related PRs, deployment notes, or "N/A">
EOF
)"
```

Replace `- [ ]` with `- [x]` for each checkbox the analysis in Step 5 justified. Leave the rest unticked.

### Step 8: Report back

Print the PR URL returned by `gh pr create` so the user can click it. If any boxes were intentionally left unticked (e.g. "tests pass locally" because tests weren't run), briefly call that out so the user knows what's still theirs to confirm.

## Rules

- **MUST run the Step 2 quality gate** before drafting, pushing, or calling `gh pr create`. If tests fail, warnings appear, or lint/format checks fail, **stop and report** — do not open the PR. The gate may only be skipped when the user explicitly instructs you to skip it in the current invocation; a prior "proceed despite" does not carry over.
- **Always** append the ticket ID in parentheses at the end of the PR title (e.g. `(SOC-277)`). The body also links the ticket — this is intentional redundancy.
- **Never** push to the project's default branch (`main`/`master`/etc.). If the current branch is the default branch, refuse and ask the user to create a feature branch.
- **Never** use `--no-verify`, `--force`, or `--force-with-lease` unless the user explicitly asks.
- Keep the template structure **verbatim**: the `## 🎫 Ticket`, `## 📝 Description`, `## ✅ Checklist`, `## 💡 Notes` headings, the `---` horizontal rules, the sub-section order, and every checkbox line. Don't add, remove, or rename sections or items.
- Only tick a checkbox when its condition is genuinely met (see Step 5). Never tick on faith.
- If `gh` is not authenticated (`gh auth status` fails), tell the user to run `! gh auth login` in the prompt rather than attempting to authenticate automatically.

## Common Mistakes

- **Skipping the quality gate** — running the gate is mandatory. A green local state is a precondition for opening a PR.
- **Hard-coding the toolchain** — detect the project's commands from its manifests and `CLAUDE.md`/`AGENTS.md`. Don't assume `pytest`/`npm`/`gradle`.
- **Forgetting the ticket ID in the title** — the title must end with `(SOC-XXX)`.
- **Ticking boxes you can't verify** — "CI passes" and "Docker builds" require real runs; leave them unticked.
- **Dropping the horizontal rules or emoji headings** — they're part of the template.
- **Summarising the diff file-by-file** instead of explaining rationale.
- **Picking the wrong ticket** when the branch has commits from multiple tickets — ask.
- **Creating a new PR when one already exists** for the branch (check with `gh pr view` first).
- **Forgetting to push the branch** before calling `gh pr create`.
- **Using local `main`** for the diff base when it's stale — prefer `origin/main` (or whatever the project's default remote branch is).
