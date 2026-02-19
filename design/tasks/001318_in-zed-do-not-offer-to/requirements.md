# Requirements: Disable Dev Container Suggestions in Zed

## User Story

As a Helix platform user, I want Zed to not prompt me to open projects in dev containers, even when a `.devcontainer` directory exists in the repository, because Helix uses its own container orchestration.

## Background

When Zed detects a `.devcontainer` directory in a project, it shows a notification asking "Would you like to re-open it in a container?" This behavior is undesirable for Helix because:

1. Helix already runs Zed inside its own managed containers
2. The suggestion creates user confusion
3. Clicking "Open in Container" would conflict with Helix's orchestration

## Acceptance Criteria

1. **Setting Added**: A new boolean setting `suggest_dev_container` is added to the `remote` settings section
2. **Default Behavior**: Setting defaults to `true` (current behavior preserved for non-Helix users)
3. **Helix Override**: When set to `false`, no dev container suggestion notification is shown
4. **No Functional Impact**: The ability to manually open dev containers via command palette remains unaffected
5. **Documentation**: Setting is documented with JSON schema for IDE autocompletion

## Out of Scope

- Removing dev container functionality entirely
- Modifying the "Open Dev Container" action behavior
- Changes to the dev container picker UI