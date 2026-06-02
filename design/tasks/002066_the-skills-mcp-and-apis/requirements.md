# Requirements: Show Session-Restart Notice in Skills/MCP Editor

## Background

The Skills editor lets users configure MCP servers and API skills. It is rendered in **two separate places**:

1. **Project settings** → Skills tab (`ProjectSettings.tsx`)
2. **Agent (App) settings** → Skills tab (`App.tsx`)

When a user adds, edits, or removes an MCP server configuration, any **currently-running session** does not automatically reload the MCP client. The user has to start a new session for the changes to take effect. Today, nothing in the UI tells the user this, so the change appears to silently fail.

## User Story

As a user editing MCP server skills in either project settings or agent settings, I want a clear notice that I'll need to start a new session to pick up my MCP configuration changes, so that I don't spend time wondering why my just-saved MCP server isn't showing up in an in-flight conversation.

## Acceptance Criteria

- [ ] When viewing the Skills tab in **Project Settings**, an informational notice is visible that says active/running sessions must be restarted to pick up MCP configuration changes.
- [ ] When viewing the Skills tab in **Agent Settings** (the App page), the same notice is visible.
- [ ] The notice is visually consistent with the existing alert/banner pattern used elsewhere in the app (MUI `<Alert>` — see the OAuth-config warning in `Skills.tsx` as a precedent).
- [ ] The notice wording explicitly mentions both **MCP servers** and **API skills** (since the editor handles both), e.g.: *"Changes to MCP servers and API skills take effect in new sessions. Restart any active session to pick up updates."*
- [ ] The notice is always visible while the Skills tab is shown (not dismissible) — this is a recurring gotcha, not a one-time onboarding message.
- [ ] No backend or API change is required.

## Out of Scope

- Auto-reloading MCP clients in running sessions (this is the underlying behavior; we are only documenting it in the UI for now).
- Wording for individual skill cards.
- Any change to the actual save flow.
