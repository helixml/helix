# Org chat: bots as top-level entries, people groups, member presence

**Date:** 2026-09-06
**Branch:** `feature/org-chat-people-bots-presence`

## What changed

The org chat sidebar (`/orgs/:org/chat`) is modelled on OpenClaw team mode and
the Grok Bot sidebar. A **Group by** picker in the toolbar (replacing the old
people filter icon) chooses one of two arrangements for the org's work — by
project or by person — and the sidebar shows exactly one of them. Org agents
sit above either arrangement:

1. **Org agents** — every helix-org agent, top level, with a green/grey status dot.
   Click opens the agent's chat session directly (`org_session`); an agent that
   has never run is started first and the open completes when the polled bots
   list carries its session id. Hover reveals an **Agent settings** gear
   (`org_agent` with the agent's app id); right-click adds Start/Stop/Restart
   and the agent page. The agent's own Helix project is no longer listed under
   Projects — it *is* the agent row. Each agent is itself collapsible and
   holds the **spec tasks it created** (`created_by_org_agent`), across every
   project the viewer can read — a multi-level tree: section → agent → tasks.
2. **Projects** (group by project) — every project with everyone's work in
   it: all members' chats (`all_members=true`) and every task, each row
   carrying the person's avatar. Chats outside any project stay personal.
3. **People** (group by person) — every member with a presence dot, the
   viewer first (expanded by default), others online first. Expanding a
   member shows their sessions and tasks across every project the viewer can
   read, flattened newest-first with the project name in the tooltip.
   Expanded members persist per user/org/project-filter in localStorage.
   Offline members beyond five are behind "Show N more offline".

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
- `GET /sessions?org_id=…&project_id=…&project_scope=project&all_members=true`
  lists every member's chats in one project; gated by project access, which is
  exactly when `authorizeUserToSession` would let the caller open them.
  `ListSessionsQuery.AnyOwner` drops the owner clause.

## Attributing tasks to agents

`SpecTask.CreatedByOrgAgent` (indexed) records the helix-org agent handle that
created a task; `CreatedBy` stays the human the agent acts for. Two creation
paths set it:

- the org runtime's MCP `create_spec_task` knows the calling worker id;
- the REST `POST /spec-tasks/from-prompt` resolves it from the caller's
  session-scoped API key: the session it names carries `org_worker_id`.

`GET /spec-tasks?created_by_org_agent=<handle>` filters on it. Tasks created
before this field exists are not attributed and do not appear under agents.

Tasks follow assignment (the store's participant filter is assignee-only by
design). A person's planning session for a task they created but assigned to
someone else is dropped from their group rather than shown as a stray chat.

## Not done / follow-ups

- Presence is per API instance in memory for the throttle; multiple API
  replicas each write at most once a minute, which is fine.
- A colleague's session opens in the normal Session page. It is readable via
  project access; the page's own owner checks decide what is editable.
