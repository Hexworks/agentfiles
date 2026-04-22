# Asset Authoring Guidelines

Assets are the reusable building blocks of a profile. They should be easy to
select, easy to understand, and safe to render into agent-specific outputs.

An asset is more than a folder of files. It is a typed, documented unit with a
stable id and a manifest that explains how it should participate in rendering.

## Always Create A Valid Manifest

Every asset must have an `asset.json` with an id, name, and supported type.

```json
// Do
{
  "id": "review",
  "name": "review",
  "type": "skill",
  "description": "Review workflow instructions"
}
```

```json
// Don't
{
  "title": "Review"
}
```

## Use Type-Specific Conventions

Type conventions reduce ambiguity and allow simpler render logic.

```text
Do:
- use SKILL.md for skill assets
- use AGENTS.md for agents_doc assets
- use codex.toml or agent-specific config files for settings assets
```

```text
Don't:
- mix unrelated file conventions in one asset without explaining them
- rely on hidden filename assumptions outside the manifest
```

## Declare Projections Explicitly

For generic asset types, use projections to map source files into managed
surfaces.

```json
// Do
{
  "projections": [
    {
      "agent": "claude-code",
      "source": "hook.sh",
      "target": ".claude/hooks/hook.sh"
    }
  ]
}
```

```json
// Don't
{
  "projections": [
    {
      "source": "hook.sh"
    }
  ]
}
```

## Keep Targets Within Managed Surfaces

Rendered outputs should only land in recognized tooling paths.

```text
Do:
- target AGENTS.md
- target .claude/, .cursor/, .codex/, .opencode/, or .mcp.json
```

```text
Don't:
- target arbitrary source files in the repository
- use projections as a generic file-copy mechanism
```

## Use Compatibility And Exclusivity When Needed

Use manifest fields to model selection rules directly.

```json
// Do
{
  "compatible_agents": ["codex"],
  "exclusive_group": "main-agents-doc"
}
```

```json
// Don't
{
  "description": "please don't combine this with the other default doc"
}
```

