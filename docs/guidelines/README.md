# Guidelines

Guidelines define how the project should be developed and documented across multiple changes. They are not architecture decisions themselves; instead they describe the preferred way to work within the architecture that the ADRs and the current codebase establish.

Whenever you make a change in the codebase try to find the relevant file(s) based on what you want to change and use the guidelines written there.

For example if you want to touch the domain model, and extend the current architecture you should look at [Clean Architecture](./clean_architecture.md) and [Domain Model](./domain_model.md).

Each guideline file contains _frontmatter_ with a `keywords` that will help you find the right one. It also contains an optional `related` field that contains paths to related markdown files.

## Available Guidelines

- [Asset Authoring](./asset_authoring.md)
- [Charm](./charm.md)
- [Clean Architecture](./clean_architecture.md)
- [Clean Code](./clean_code.md)
- [Documentation](./documentation.md)
- [Domain Model](./domain_model.md)
- [Errors](./errors.md)
- [External Tools](./external_tools.md)
- [Git](./git.md)
- [Go](./go.md)
- [Security](./security.md)
- [SOLID](./solid.md)
- [Sync And Safety](./sync_and_safety.md)
- [Testing](./testing.md)
- [TUI](./tui.md)
