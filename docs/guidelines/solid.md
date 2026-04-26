# SOLID Guidelines

SOLID is a set of design checks for keeping code understandable, changeable, and
testable. In `agentfiles`, use these principles pragmatically. Prefer the
existing package structure and local conventions over pattern completeness.

Apply SOLID when it makes the next change clearer. Do not add interfaces,
layers, or indirection only because a principle can be named.

## Single Responsibility Principle

A package, type, or function should have one clear reason to change. A "reason"
is a source of change, such as a domain rule, rendering behavior, filesystem
sync behavior, command handling, or presentation.

Keep unrelated reasons to change apart. For example, domain validation should
not also know how to print CLI output, and rendering should not also decide
which assets belong to a project.

Use this principle when a change forces edits in a place that should not know
about that concern. Split code only when the split gives the concern a clearer
owner.

## Open/Closed Principle

Code should be easy to extend for a new case without rewriting stable behavior.
This does not mean every feature needs a plugin system or an interface.

Prefer small, explicit extension points where variation is already real: a new
renderer mode, asset type, output target, or side-effect adapter. Keep the
stable rule closed by testing it and by adding new behavior around it instead of
editing the rule repeatedly.

Modify existing code when the model itself is wrong or incomplete. Extending a
bad abstraction usually makes the design harder to understand.

## Liskov Substitution Principle

Any implementation used through an interface or shared contract must preserve
the behavior callers expect. In Go, this applies to interfaces, structs used in
the same role, test fakes, and adapters.

Do not make a caller check which concrete implementation it received before it
can use the value safely. If an implementation cannot honor the contract, either
narrow the contract or create a separate one.

Test fakes should follow the same important rules as production
implementations. A fake that accepts impossible state can hide bugs instead of
making tests easier to trust.

## Interface Segregation Principle

Callers should depend only on the behavior they actually use. Keep interfaces
small and define them near the package that consumes them when that package
needs to invert a dependency.

Avoid broad interfaces that combine reading, writing, validation, rendering, and
syncing in one contract. Split them when callers need only one part, or pass a
concrete type when there is no useful abstraction.

Use this principle to reduce coupling, not to create an interface for every
struct.

## Dependency Inversion Principle

Stable policy should not depend on volatile technical details. Domain and
application rules should not import CLI, filesystem, environment, rendering
tooling, or external command details unless that is already the established
boundary.

When a stable rule needs a side effect, pass the side effect in as a narrow
interface or function owned by the caller. Keep wiring near the edge where the
concrete implementation is chosen.

Use dependency inversion to make side effects explicit and tests simpler. Avoid
it when a direct call is clearer and the dependency is already stable.

## Applying SOLID

Before changing code, ask:

1. What responsibility is being changed?
2. Which package or type owns that responsibility today?
3. Would this change be clearer as a direct edit, a new case, or a new boundary?
4. Are callers forced to know details they should not know?
5. Can the behavior be tested without unrelated infrastructure?

The goal is code where the important rule is easy to find, the dependency
direction is obvious, and the next related change stays local.
