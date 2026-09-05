# Org chat: bots as top-level entries, people groups, member presence

**Date:** 2026-09-06
**Branch:** `feature/org-chat-people-bots-presence`

## What changed

The org chat sidebar (`/orgs/:org/chat`) now has three sections, modelled on
OpenClaw team mode and the Grok Bot sidebar:

1. **Bots** — every helix-org agent, top level, with a green/grey status dot.
   Click opens the agent's chat session directly (`org_session`); an agent that
   has never run is started first and the open completes when the polled bots
   list carries its session id. Hover reveals an **Agent settings** gear
   (`org_agent` with the agent's app id); right-click adds Start/Stop/Restart
   and the agent page. The agent's own Helix project is no longer listed under
   Projects — it *is* the bot row.
2. **Projects** — unchanged: the viewer's own tasks and chats grouped by project.
3. **People** — every other org member with a presence dot, online first.
   Expanding a member shows their work: their sessions and tasks across every
   project the viewer can read, flattened newest-first with the project name in
   the tooltip. Expanded members persist in the same localStorage slot the old
   people filter used; the filter popover (search for large orgs) now lives in
   the People header and toggles the same set. Offline members beyond five are
   behind "Show N more offline".

The **org People page** shows the same presence dot per member.

## Presence

`users.last_seen_at` already existed (auth middleware, throttled). The throttle
dropped from 5 min to 1 min, and `types.PresenceOnlineWindow` (3 min) defines
online. The members list handler computes `OrganizationMembership.Online`
(`gorm:"-"`) server-side so client clock skew cannot matter. The sidebar and
People page poll `GET /organizations/{id}/members` every 30 s via
`useOrganizationMembers`; an open Helix tab keeps polling other endpoints, so
"online" means "has Helix open".

## Seeing another member's work

- `GET /sessions?org_id=…&owner_id=<user>` lists another member's sessions.
  Mirrors `authorizeUserToSession`: org owners see everything, other members
  only sessions inside projects they can read (`visibleOrganizationProjects`,
  shared with the projects list). Sessions with no project are invisible to
  non-owners. `ListSessionsQuery.RestrictToProjects/ProjectIDs` carries the
  bound; an empty bound matches nothing.
- `GET /spec-tasks?organization_id=…&participant_ids=<user>` lists tasks across
  every readable project (`SpecTaskFilters.FilterProjectIDs/ProjectIDs`).
  `project_id` is no longer required when `organization_id` is given.
- Bot list DTO gained `project_id` and `session_id` (from runtime state, already
  loaded per bot) so the sidebar needs no per-bot detail fetch.

## Not done / follow-ups

- Presence is per API instance in memory for the throttle; multiple API
  replicas each write at most once a minute, which is fine.
- A colleague's session opens in the normal Session page. It is readable via
  project access; the page's own owner checks decide what is editable.
