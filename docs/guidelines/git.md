# Git Guidelines

Git history in `agentfiles` should make the purpose of each change easy to
understand and easy to review. The project uses one long-lived `main` branch
with short-lived topic branches that are rebased before they are merged back.

Commit messages should follow
[Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
so the history communicates intent consistently to humans and tooling.

## Use Short-Lived Topic Branches

Create a branch from the latest `main` for each focused change. Use a short,
lowercase, hyphenated name with a prefix that describes the work.

```text
Do:
- branch from an up-to-date main branch
- use names like feature/profile-registry, bug/sync-drift-check, docs/git-guidelines
- keep each branch focused on one task or tightly related set of changes
- merge completed topic branches back into main
```

```text
Don't:
- commit feature work directly on main
- reuse old branches for unrelated work
- mix unrelated fixes, refactors, and documentation updates in one branch
```

## Rebase Before Merge

Keep topic branches current by rebasing them onto `main`. Prefer rebasing over
merging `main` into the branch so the commit log stays linear and readable.

```text
Do:
- fetch the latest remote state before rebasing
- run git rebase origin/main before merge or review handoff
- resolve conflicts by preserving the intended behavior from both sides
- rerun the relevant checks after conflict resolution
```

```text
Don't:
- create merge commits just to update a topic branch
- rebase public branches other people are actively building on without
  coordination
- force-push over someone else's work
```

## Write Conventional Commits

Use this commit message shape:

```text
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

The type is required. Use `feat` for user-visible features and `fix` for bug
fixes. Other useful types include `docs`, `test`, `refactor`, `perf`, `build`,
`ci`, `chore`, `style`, and `revert`.

```text
Do:
- write small commits that each explain one coherent change
- use an optional scope when it clarifies the affected area
- keep the description short and specific
- add a body when the commit needs context, tradeoffs, or migration notes
- use footers for issue references and other metadata
```

```text
Don't:
- use vague messages like update, fixes, or changes
- hide unrelated work inside one broad commit
- describe only the files changed instead of the behavior changed
- add a body when the subject line already says enough
```

## Mark Breaking Changes Clearly

Breaking changes must be visible in the commit header or footer. Add `!` before
the colon or include a `BREAKING CHANGE:` footer with the required explanation.

```text
Do:
- feat(config)!: replace profile registry format
- refactor(sync): simplify apply planning

  BREAKING CHANGE: managed state files must be regenerated after upgrading.
```

```text
Don't:
- bury a compatibility break only in the body
- use breaking-change markers for internal refactors that do not affect callers,
  users, data formats, or documented behavior
```

## Split Commits By Intent

If a change naturally needs more than one commit type, split it when possible.
This keeps review focused and makes release notes more accurate.

```text
Do:
- feat(tui): add project filter
- test(tui): cover project filter behavior
- docs: document project filtering
```

```text
Don't:
- commit unrelated code, tests, and docs as chore: updates
- use one large commit when separate commits would tell the story better
```
