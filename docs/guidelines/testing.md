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
