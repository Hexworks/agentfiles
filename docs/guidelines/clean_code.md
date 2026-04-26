# Clean Code Guidelines

Clean code is code that a future maintainer can understand, change, and verify
without reconstructing the whole system first. In `agentfiles`, clean code means
clear domain intent, small moving parts, explicit side effects, and tests that
protect the behavior being changed.

These rules are practical guidance for human contributors and coding agents.
Prefer the existing style of the codebase over generic advice when the two
conflict.

## General Rules

1. Follow local conventions before introducing a new pattern.
2. Prefer the simplest design that solves the current problem well.
3. Keep changes focused on the requested behavior.
4. Fix the root cause when it is visible; do not hide a broken model behind a
   narrow workaround.
5. Leave touched code clearer than you found it, without doing unrelated
   refactors.

## Design Rules

1. Keep domain rules separate from command handling, file I/O, and external
   integrations.
2. Keep configuration and environment details at the edges of the system.
3. Use dependency injection when it makes side effects explicit or tests easier.
4. Use interfaces or polymorphism only when they simplify a real variation.
5. Avoid over-configurability. A clear fixed rule is better than a vague option.
6. Isolate concurrency, synchronization, and filesystem mutation so they are
   easy to reason about.

## Understandability

1. Make intent visible in names, types, and function boundaries.
2. Be consistent. Similar concepts should look similar across packages.
3. Use explanatory variables when they make a condition or transformation easier
   to read.
4. Put boundary checks and edge-case handling in one obvious place.
5. Prefer positive conditions over negative ones when both are equally correct.
6. Avoid hidden ordering requirements between methods or package-level state.

## Naming

1. Choose names that describe domain meaning, not just data shape.
2. Make meaningful distinctions. Avoid names that differ only by vague words like
   `data`, `info`, `manager`, or `helper`.
3. Use names that are easy to search for and pronounce.
4. Replace magic numbers and strings with named constants when the value has
   meaning outside one local expression.
5. Avoid encoding type or scope into names unless it is an established Go
   convention.

## Functions

1. Keep functions small enough that their purpose is obvious.
2. Let each function do one coherent job at one level of abstraction.
3. Give functions names that describe the behavior they provide.
4. Prefer a few meaningful arguments over long parameter lists.
5. Avoid surprising side effects. If a function mutates files, state, or inputs,
   make that visible in its name, package, or return value.
6. Avoid flag arguments that switch between unrelated behaviors. Split the
   behavior when separate callers need separate modes.

## Comments

1. First try to make the code explain itself with better names or structure.
2. Use comments to explain intent, constraints, tradeoffs, or consequences.
3. Do not repeat what the next line of code already says.
4. Do not keep commented-out code. Remove it.
5. Do not add closing-brace comments or other visual noise.
6. Comment non-obvious invariants, safety rules, and compatibility requirements.

## Source Structure

1. Keep related code close together.
2. Declare variables close to where they are used.
3. Put dependent functions near each other when that improves reading flow.
4. Keep package boundaries aligned with domain concepts.
5. Use whitespace to group related steps and separate different concerns.
6. Keep indentation and formatting standard; let the language formatter win.
7. Avoid horizontal alignment that makes future edits noisy.

## Data And Objects

1. Prefer explicit types for stable domain concepts.
2. Hide internal structure when callers should not rely on it.
3. Use plain data structures when the value is only data.
4. Avoid hybrids that expose fields while also requiring callers to know hidden
   behavior.
5. Keep structs focused. Too many fields usually means the concept is doing too
   much.
6. Prefer behavior on the package or type that owns the rule.

## Tests

1. Test behavior, not implementation details.
2. Keep each test focused on one rule or outcome.
3. Make tests readable: clear setup, one action, visible assertions.
4. Keep tests fast, isolated, and repeatable.
5. Use stable test data and `t.TempDir()` for filesystem work.
6. Add or update tests when changing domain rules, synchronization behavior, or
   error handling.

## Code Smells

1. Rigidity: a small change forces many unrelated changes.
2. Fragility: one change breaks distant behavior.
3. Immobility: useful code cannot be reused because it is tangled with context.
4. Needless complexity: abstractions, options, or layers that do not pay for
   themselves.
5. Needless repetition: duplicated rules that can drift apart.
6. Opacity: code that works only after the reader reverse-engineers it.

When in doubt, optimize for the next correct change: make the behavior explicit,
keep the edit local, and leave enough tests or structure for the next maintainer
to trust it.
