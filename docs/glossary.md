# Domain Glossary

This glossary defines the canonical vocabulary for the `agentfiles` bounded
context. The project uses these terms to describe how reusable LLM workspace
content is modeled, selected, rendered, and synchronized into repositories.

Following the bounded-context idea, the glossary prefers one precise meaning for
each term inside this project. If implementation details evolve, the glossary
should be updated so the language stays internally consistent.

## Registry

The global profile index stored at `~/.agentprofiles.json`. It contains profile
references and is used for discovery and resolution.

## Profile

A root folder containing `profile.json`, `assets/`, and `projects/`. It is the
main source-of-truth unit in the system.

## Profile Reference

A registry entry that describes a profile by id, name, path, source,
`managed_by`, and timestamps.

## Profile Manifest

The `profile.json` file at the root of a profile. It records stable metadata
about that profile.

## Project

A target repository together with a per-profile selection of enabled agents and
selected assets.

## Project Manifest

A JSON file under `projects/<id>.json` that records the project path, enabled
agents, selected asset ids, and metadata such as creation time.

## Asset

A reusable, profile-scoped unit of content with an `asset.json` manifest and
optional files.

## Asset Type

The first-class category of an asset. Current types are `skill`, `agents_doc`,
`settings`, `mcp`, `rule`, and `hook`.

## Selected Asset

An asset that has been explicitly attached to a project through the project's
manifest.

## Enabled Agent

An LLM tool that the project should render for. Current names are `codex`,
`claude-code`, `cursor`, and `opencode`.

## Compatible Agents

An optional asset manifest field that limits which enabled agents can use that
asset.

## Projection

A mapping from an asset source file or directory to an agent-specific target
path.

## Exclusive Group

An asset manifest key that marks assets as mutually exclusive so only one chosen
asset from that group may render for a project.

## Render Plan

The computed desired output set for a project after selected assets and enabled
agents are resolved.

## Preview

The sync-layer representation of pending changes, including creates, updates,
drift, and delete candidates.

## Managed Surfaces

The limited set of output locations that `agentfiles` is allowed to manage:
`AGENTS.md`, `.claude/`, `.cursor/`, `.codex/`, `.opencode/`, and `.mcp.json`.

## Managed State

The `.agentfiles/state.json` file written into a target repository. It stores
managed-file hashes and generation metadata for the last successful apply.

## Drift

A condition where a previously managed file was changed locally after apply and
now differs from the managed-state hash.

## Delete Candidate

A recognized LLM-tooling file in a managed surface that is present in the
repository but not in the current desired output set.

## Apply

The act of writing a preview's desired outputs into a target repository and then
persisting fresh managed state.

## Source Of Truth

The authoritative location for reusable content and selection state. In
`agentfiles`, this is the profile folder, not the generated project files.

## Project Ownership

The rule that one target project path may belong to only one profile.
