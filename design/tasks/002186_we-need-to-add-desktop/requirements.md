# Requirements: Add Desktop View to the Bot Detail Page

## Background

The spec-task detail page already embeds a live remote-desktop widget that
streams the agent's screen and accepts keyboard/mouse input
(`ExternalAgentDesktopViewer`, used on `SpecTaskDetailContent` and ~12 other
call sites: `TeamDesktopPage`, `SandboxDesktopTab`, `ProjectSettings`, etc.).

Each Bot in the helix-org subsystem runs its own "Human Desktop" agent
session in a desktop container (the Bot detail page already resolves that
session and shows it as an **inline chat transcript**). What it does **not**
show is the bot's actual desktop — the same streamed screen you get on the
spec-task page.

The Bot view referenced is, e.g.,
`/orgs/helix/helix-org/bots/w-docs-engineer`. Note: the Role/Worker → **Bot**
merge is in flight in task **002185**; this feature lands on the merged
`HelixOrgBotDetail.tsx` (see Design for the dependency/conflict note).

## Goal

Add a **Desktop view** to the Bot detail page, reusing the existing
`ExternalAgentDesktopViewer` widget, driven by the bot's own agent session —
so an operator can watch and drive the bot's desktop without leaving the
bot page.

## User Stories

### US-1 — Operator watches a bot's desktop
As an org operator, I want to see the bot's live desktop on its detail page
so I can observe what its agent is doing in real time.
- **AC1** The Bot detail page offers a **Desktop** view alongside the
  existing inline **Chat** transcript (e.g. a Chat | Desktop toggle on the
  session panel).
- **AC2** Selecting **Desktop** renders `ExternalAgentDesktopViewer` in
  `mode="stream"` bound to the bot's resolved session.
- **AC3** The widget shows the same lifecycle states it shows elsewhere
  (starting / running / paused), reusing the component's built-in handling —
  no bespoke state UI.

### US-2 — Desktop binds to the bot's session
As the React client, I want the Desktop view to attach to the bot's existing
"Human Desktop" session.
- **AC1** The Desktop view uses the **same session id** the Bot page already
  resolves for the inline chat (GET-only via the project's exploratory
  session — opening the page must never provision new infra).
- **AC2** Display resolution / FPS are taken from the bot's agent app
  (`external_agent_config`: `display_width`/`display_height`/`resolution`/
  `display_refresh_rate`), falling back to 1920×1080×60 when absent — matching
  the spec-task page's derivation.

### US-3 — Graceful empty / unavailable state
As an operator, when the bot has no running desktop yet, I want a clear empty
state instead of a broken viewer.
- **AC1** When no session is resolved, the Desktop view shows an empty/idle
  message (parity with the existing "No conversation yet" chat empty state),
  not an error.
- **AC2** A human-kind participant (if still applicable post-002185) with no
  agent desktop does not show a Desktop view.

## Out of Scope
- No new backend endpoints — the existing external-agent WS stream/input
  endpoints are reused unchanged.
- No changes to the bot lifecycle, activation, or session provisioning.
- No new desktop-streaming features (input, multiplayer, etc. already exist).
