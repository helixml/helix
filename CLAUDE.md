# Claude Rules for Helix Development

The following rule files should be consulted when working on the codebase:

@.cursor/rules/helix.mdc
@.cursor/rules/go-api-handlers.mdc
@.cursor/rules/use-gorm-for-database.mdc
@.cursor/rules/use-frontend-api-client.mdc

This file contains critical development guidelines and context that MUST be followed at all times during Helix development.

## Documentation Organization

**CRITICAL: LLM-generated design documents MUST live in `design/` folder ONLY**

- **`design/`**: All LLM-generated design docs, architecture decisions, status reports, debugging logs
  - These are internal development artifacts created during the feature development process
  - Naming convention: `YYYY-MM-DD-descriptive-name.md` (e.g., `2025-09-23-wolf-streaming-architecture.md`)
  - Date should reflect when the document was written, enabling chronological navigation

- **`docs/`**: External user-facing documentation ONLY
  - User guides, API documentation, deployment instructions
  - Documentation meant for external consumption
  - Should NOT contain LLM-generated design artifacts

- **Root level**: Only `README.md`, `CONTRIBUTING.md`, and `CLAUDE.md` (this file)

**Why this matters:**
- Keeps internal design artifacts separate from user-facing documentation
- Dates in filenames provide clear chronological history of development decisions
- Makes it easy to follow the evolution of architectural decisions over time

## Hot Reloading Development Stack

The Helix development stack has hot reloading enabled in multiple components for fast iteration:

- **Frontend**: Vite-based hot reloading for React/TypeScript changes
- **API Server**: Air-based hot reloading for Go API changes
- **GPU Runner**: Live code reloading for runner modifications

This means you often don't need to rebuild containers - just save files and changes are picked up automatically.
