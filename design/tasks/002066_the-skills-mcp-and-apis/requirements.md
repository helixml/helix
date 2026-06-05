# Requirements: Rename Skills → MCPs & APIs and Add Session-Restart Notice

## Background

The "Skills" editor in helix configures two kinds of agent tooling: **MCP servers** and **HTTP APIs**. It's rendered in two places:

1. **Project settings** → Skills tab (`ProjectSettings.tsx`)
2. **Agent (App) settings** → Skills tab (`App.tsx`)

Two problems with this UI today:

1. **Name collision with Anthropic Skills.** Helix's "Skills" predates Anthropic's Skills feature. Now that Anthropic Skills exist, users think the helix Skills tab is for managing those, and don't find the place where they configure MCP servers.
2. **Silent staleness on save.** When a user changes an MCP server config, currently-running sessions keep using the old config until they're restarted. The UI gives no hint of this, so users assume changes are live and waste time debugging.

This task addresses both.

## User Stories

### Story 1 — Rename for discoverability

As a user looking for where to configure my MCP servers, I want the relevant settings tab to obviously say "MCP" in its name, so that I find it on the first try instead of overlooking it under "Skills" (which I now associate with the Anthropic feature).

### Story 2 — Session-restart notice

As a user editing my MCPs and APIs in either project settings or agent settings, I want a clear notice that already-running sessions need to be restarted to pick up my changes, so I don't spend time wondering why my just-saved MCP isn't showing up in an in-flight conversation.

## Acceptance Criteria

### Naming

- [ ] The user-visible label everywhere "Skills" appears today is replaced with **"MCPs & APIs"**. This covers tab labels, headings, dialog titles, button text, sidebar items, empty-state text, and tooltips.
- [ ] Internal identifiers (component names, types, constants, variable names, file names, Go package names) are also renamed away from `skill` / `Skill` to use `mcpAndApi` / `MCPAndAPI` (or the snake/kebab variants where context requires). Code and UI use the same vocabulary.
- [ ] The REST endpoint `/api/v1/skills` is replaced with `/api/v1/mcps-and-apis`. For one release, the old path returns a 308 redirect (or remains as a deprecated alias) so any in-flight integrations don't break instantly; a deprecation notice is added to the swagger spec.
- [ ] The response field name `"skills": [...]` is renamed to `"mcpsAndApis": [...]`. (Same backward-compat handling as above for one release.)
- [ ] The URL slug `?tab=skills` is renamed to `?tab=mcps-and-apis`. Existing bookmarks/links using `?tab=skills` are redirected client-side to the new slug for one release.
- [ ] Frontend API client types are regenerated from the updated swagger spec and consumed throughout the frontend.
- [ ] Documentation in `/docs/` and `/helix/README.md` that mentions "skills" is updated.

### Session-restart notice

- [ ] When the **MCPs & APIs** tab is open in **Project Settings**, an informational MUI `<Alert severity="info">` is visible above the editor body with wording like: *"Changes to MCPs and APIs take effect in new sessions. Restart any active session to pick up updates."*
- [ ] The same notice appears in **Agent Settings → MCPs & APIs**.
- [ ] The notice is always visible (not dismissible) — this is a recurring constraint, not a one-time message.
- [ ] No backend / API change is required for the notice itself.

## Out of Scope

- Auto-reloading MCP clients in running sessions (we are documenting the existing behavior, not changing it).
- Splitting the editor into two separate tabs ("MCPs" and "APIs"). The unified editor stays; only the label changes.
- Renaming Anthropic's Skills feature integration (if any exists separately) — that genuinely is "skills" in Anthropic's sense.
- Migrating any persisted user data — skills are runtime objects loaded from YAML and `app.config`, not a separate DB table, so no schema migration is needed (per code survey).

## Alternative naming considered

The user asked whether there's "a better generic term" than "MCPs & APIs". Candidates were surveyed:

| Candidate | Why rejected |
| --- | --- |
| Integrations | Already used in this codebase (`IdeIntegrationSection`, `ZapierIntegrations`, `ApiIntegrations`) — would collide. |
| Capabilities | Already used as a sidebar section title ("Agent Capabilities") — would collide. |
| Tools | Already used to mean LLM tool-use definitions (`TypesToolBrowserConfig`, etc.) — would collide. |
| Connectors | Free, clean, but loses the MCP discoverability that's the explicit driver of this task. |
| MCPs & APIs | Verbose, but **explicitly contains "MCP"** which is what users search for, and avoids every collision above. **Selected.** |
