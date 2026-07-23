# Testing Guidelines

Tests in `agentfiles` should make behavior easy to trust and failures easy to
understand. Prefer small, direct tests that describe the domain rule being
verified over broad tests that only prove that code ran.

Use the [testing pyramid](https://martinfowler.com/articles/practical-test-pyramid.html)
as the default ordering principle: write unit tests first, add service or
integration tests when real collaboration between packages or filesystem state
matters, and keep end-to-end tests rare and focused on critical workflows.

## Start With The Smallest Useful Test

Write the cheapest test that can prove the behavior.

```text
Do:
- unit-test domain rules and pure transformations first
- use service or integration tests for package collaboration and real I/O
- use end-to-end tests only for important user workflows
```

```text
Don't:
- start with broad workflow tests when a unit test can prove the same rule
- duplicate the same behavior at every layer of the pyramid
```

## Test One Behavior At A Time

Each test should make one promise about the system. If the test fails, the name
and assertions should make the broken behavior obvious.

```text
Do:
- name tests after the behavior and expected outcome
- keep setup focused on the behavior under test
- split unrelated expectations into separate tests
```

```text
Don't:
- hide several independent rules inside one large test
- use names that only repeat the function name without saying what matters
```

## Separate Given, When, Then

Make the test flow visible. Use blank lines or short comments when that improves
readability.

```text
Do:
- given: create the required state
- when: execute the behavior once
- then: assert the observable result
```

```text
Don't:
- mix setup, execution, and assertions throughout the test
- create state that is not needed for the expected result
```

## Assert Behavior, Not Mock Mechanics

Unit tests should control dependencies with simple stubs, fakes, or mocks when
those dependencies are slow, stateful, external, or hard to make deterministic.
The assertion should still be about the behavior implemented by the unit.

```text
Do:
- stub dependencies so the unit test is deterministic
- after creating a record, assert that finding it returns the expected record
- verify calls only when the collaboration itself is the behavior
```

```text
Don't:
- test that a mock's create method was called once as proof that creation works
- mock the code path that the test is supposed to verify
- add a mocking framework when a small hand-written fake is clearer
```

## Cross-Boundary Integration

Any change that mutates files, spawns processes, or crosses a package
boundary that owns a layout, schema, or protocol must be covered by at
least one **full-stack real** test — no fakes anywhere between the entry
point and the effect.

```text
Do:
- exercise real domain constructors (e.g. real Init/Load) rather than
  hand-seeded fixtures whose shape the test author chose
- shell out to the real external tool (git, subprocess, the OS
  filesystem) inside a t.TempDir() sandbox
- assert against ground truth: file contents on disk, `git log`
  output, the actual bytes written, HTTP response — not against the
  string constant the implementation also produced
- when path handling is involved, add a non-default-placement variant
  (nested folder, symlink, non-repo-root, non-cwd) so path assumptions
  cannot ride the "everything at the top" happy path
```

```text
Don't:
- write a test whose expected value is a copy of the string constant
  the code under test built. If both sides can drift together, the
  test proves nothing.
- rely only on fake-recorded-tuple assertions when the tuple is the
  contract with an external system. Fake tests belong next to real
  tests, never instead of them.
- skip the real-stack test because it needs the git binary or a
  scratch directory. Skip on binary absence (`exec.LookPath`) so the
  test degrades cleanly, but do not delete it.
```

Concrete triggers that require a real-stack test:

- Any use of `os/exec`, `exec.Command`, subprocess handoff.
- Any use of `filepath.Join`, `filepath.Rel`, `filepath.Abs`, path
  glob or pathspec strings in production code.
- Any code that computes a repo-relative or root-relative path.
- Any code that reads or writes a serialization format the tool did
  not author (git object database, external config file, third-party
  API payload).

Rationale: task 0042 shipped with every unit test green because the
fake committer's expected pathspec was a copy of the implementation's
own wrong constant, and the only real-git tests used a fixture whose
layout matched the wrong constant. The bug surfaced on the first live
run. See
[`docs/changelog/2026-07-23_0042-git-aware-commits.md`](../changelog/2026-07-23_0042-git-aware-commits.md).

## Keep Tests Isolated

Tests must not depend on execution order or state left by another test. Prefer
`t.TempDir()` for filesystem tests and stable, hard-coded identifiers for
domain objects created by the test.

```text
Do:
- use stable ids that are unique to the test
- clean known leftover state before setup when shared resources are unavoidable
- create all required state in the given step
- clean up after the test using the same stable ids
```

```text
Don't:
- rely on data created by another test
- use random identifiers when a stable id would make failures easier to debug
- leave shared state that can change another test's result
```

## Keep Test Code Simple

Test code should be boring. A little duplication is better than abstractions
that make each test harder to read.

```text
Do:
- write direct setup for simple cases
- extract helpers only when they remove meaningful noise
- keep helpers small and behavior-specific
```

```text
Don't:
- introduce inheritance or deep fixture layers
- hide the important parts of given, when, or then inside generic helpers
```
