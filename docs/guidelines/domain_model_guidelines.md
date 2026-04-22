# Domain Model Guidelines

The `agentfiles` domain model should stay explicit and stable because it is the
main source of safety in the project. The application is not a generic sync
engine; it is a focused system for profile-based LLM workspace composition and
rendering.

When the model grows, prefer adding well-named concepts instead of overloading
existing ones. A good rule is that registry, profile, asset, project, render,
and sync should remain separable both in code and in documentation.

## Keep Boundaries Clear

Each domain concept should have one primary responsibility.

```text
Do:
- use registry for discovery metadata
- use profiles for reusable source content
- use projects for per-repository selection
- use render for desired outputs
- use sync for comparison and writes
```

```text
Don't:
- turn the registry into a second source of project truth
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

