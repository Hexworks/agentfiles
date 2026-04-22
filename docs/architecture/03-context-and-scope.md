# 3. Context And Scope

`agentfiles` sits between a user's profile library and one or more target source
repositories. Its responsibility is to resolve selected assets, transform them
into agent-specific outputs, and synchronize those outputs safely.

## System Context

```text
+------------------+        +----------------------+
| User / Terminal  | -----> | agentfiles CLI / TUI |
+------------------+        +----------------------+
           |                           |
           |                           +----> ~/.agentprofiles.json
           |                           |
           |                           +----> Profile folders
           |                           |      profile.json
           |                           |      assets/
           |                           |      projects/
           |                           |
           |                           +----> Target repositories
           |                                  AGENTS.md
           |                                  .claude/
           |                                  .cursor/
           |                                  .codex/
           |                                  .opencode/
           |                                  .agentfiles/state.json
```

## External Interfaces

### User Interface

The system is fully TUI-driven. Running `af` with no arguments opens the
top-level menu; subcommand paths (`profile create`, `asset init`,
`project add`, `project plan`, `project apply`, `doctor`) jump straight to
the matching form. There is no standalone `tui` command — the TUI is the
only interactive surface.

### Global Registry

The registry file at `~/.agentprofiles.json` is used to discover profiles and
resolve them by id, name, or path.

### Filesystem

Profiles and projects are both normal folders. This is the dominant external
boundary of the application.

