# Domain Model Guidelines

The `agentfiles` domain model should stay explicit and stable because it is the
main source of safety in the project. The application is not a generic sync
engine; it is a focused system for profile-based composition of LLM workspace
files into target repositories.

Use domain-driven design as a practical discipline: shape code, tests,
documentation, and user flows around the problem the project actually solves.
Domain modeling should make rules easier to see and change; it should not add
layers, factories, or patterns that do not carry domain meaning.

When the model grows, prefer adding well-named concepts instead of overloading
existing ones. Registry, profile, asset, project, render, and sync should remain
separable both in code and in documentation.

Related: [Clean Architecture](./clean_architecture.md)

## Use The Project Language

Prefer the canonical vocabulary from the glossary and existing packages.

```text
Do:
- use profile, project, asset, render plan, preview, drift, and managed state
- name functions and tests after domain behavior
- update the glossary when a new durable domain term appears
```

```text
Don't:
- invent parallel names for established concepts
- hide domain rules behind vague names like data, item, manager, or handler
- let UI labels, docs, and code drift into different meanings
```

## Keep Boundaries Clear

`agentfiles` has one focused bounded context: reusable AI-assistant
configuration is selected from profiles and rendered into repositories. Inside
that context, each domain concept should have one primary responsibility.

```text
Do:
- let registry handle discovery metadata
- let profile and asset model source content
- let project model per-repository selection
- let render compute desired outputs
- let sync compare and apply those outputs
```

```text
Don't:
- make render read repository state as input
- make sync decide which assets belong to a project
- turn registry into a second source of profile or project truth
- store rendered outputs inside profile manifests
```

## Preserve Source-Of-Truth Semantics

Profile content is authoritative. Generated project files are outputs.

```text
Do:
- model project-side changes as drift
- keep assets and manifests in profile folders
```

```text
Don't:
- let generated files silently redefine profile assets
- blur the distinction between source content and outputs
```

## Put Domain Rules In Domain Code

Rules that protect the model should live near the model they constrain, not as
one-off checks in the TUI or orchestration layer.

```text
Do:
- enforce stable ids in manifest validation
- enforce compatible_agents and exclusive_group through render behavior
- enforce single project ownership in the application boundary
- keep managed-surface safety checks in the rendering and sync path
```

```text
Don't:
- rely on screen-level validation as the only protection
- duplicate the same rule in several command or form handlers
- encode durable business rules only in comments or README text
```

## Model Consistency Boundaries

Treat consistency boundaries as the places where invariants must be preserved
together. They do not need ceremony, but callers should update them through code
that can validate the whole boundary.

```text
Do:
- treat a profile manifest and its assets as one source-of-truth boundary
- treat a project manifest as the boundary for selected agents and assets
- keep one invariant from being split across several partial updates
```

```text
Don't:
- let unrelated packages mutate manifest fields directly
- create aggregate roots just to mirror every struct in the codebase
- add repositories or factories by default
```

## Use Stable Identifiers

Profiles, projects, and assets need stable ids because selection and ownership
depend on them.

```json
// Do
{
  "id": "review",
  "name": "review",
  "type": "skill"
}
```

```json
// Don't
{
  "name": "Review Skill"
}
```

## Model Constraints Explicitly

Compatibility and exclusivity rules belong in the domain model, not in ad hoc
command checks.

```json
// Do
{
  "compatible_agents": ["codex", "claude-code"],
  "exclusive_group": "primary-instructions"
}
```

```json
// Don't
{
  "notes": "only use this with codex unless another file says otherwise"
}
```

## Keep Application Flow Thin

Application services should coordinate use cases. They should not become the
place where every domain rule is implemented.

```text
Do:
- keep app services thin and explicit
- call domain packages for validation, rendering, planning, and applying
- keep TUI code focused on input, confirmation, and result presentation
```

```text
Don't:
- put rendering rules in the TUI
- put file-write policy in asset selection logic
- make orchestration code depend on accidental JSON shapes
```

## Keep Persistence A Detail

JSON files are the current storage mechanism, not the domain itself. Code should
prefer explicit domain structs and behavior over loose file-shaped maps.

```text
Do:
- parse JSON into named domain types
- validate manifests after loading
- keep file paths and timestamps at the edges unless they are part of the rule
```

```text
Don't:
- pass map[string]any through business logic
- make domain behavior depend on incidental formatting in JSON files
- treat storage layout as a substitute for a clear model
```

## Be Pragmatic

Use domain-modeling patterns only when they clarify real complexity.

```text
Do:
- introduce value objects for concepts with rules, such as safe target paths
- introduce services when behavior does not naturally belong to one entity
- keep simple data simple when it has no meaningful behavior
```

```text
Don't:
- force every struct to have methods if validation is enough
- add domain events before a real workflow consumes them
- make the code harder to follow in the name of pattern completeness
```
