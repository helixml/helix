# Requirements: Filter Helix-Org-Chart and Non-External Agents from Spec Task Agent Switchers

## Background

The spec task details page (`SpecTaskDetailContent.tsx`) has **two** agent-picker
dropdowns, both built from the same apps context (`useApps()` → `GET /api/v1/apps`) and
rendered with the shared `AgentDropdown` component:

1. **Live-switch control (primary target)** — `SwitchAgentControl`, mounted at the top of
   the chat panel header. The dropdown Kai recently added that switches the *running
   session* to a different agent **in place** (same session id, same container, transcript
   preserved). See `design/tasks/002111_so-we-recently-added-a/`. It already filters to
   `zed_external` agents.
2. **Details-panel agent selector (secondary target)** — the `AgentDropdown` in the details
   sidebar that sets the task's assigned agent (`helix_app_id`). Currently it only *sorts*
   external agents to the top and still lists non-external ones.

We want both dropdowns to offer only agents that make sense for switching a spec task:
standalone **external (`zed_external`) agents**, excluding (a) non-external agents and
(b) agents that belong to the **Helix Org chart**.

## Clarification: what "Helix Org" means here

"Helix Org" refers to **Phil's new org-chart-of-agents feature** (gated by
`HELIX_ORG_ENABLED`), NOT a normal customer Organization. In that feature you define an
**org chart of agents** ("Workers") that report to / delegate to each other. Each AI Worker
is backed by a Helix App, linked via the org runtime-state table
(`org_worker_runtime_state`, key `agent_app_id`).

These org-chart Worker agents must be excluded from the spec task switchers. Importantly:
- A spec task agent may legitimately be owned by a customer organization
  (`owner_type === 'org'`). That is fine — those must **still appear**.
- Only agents that are part of the **Helix Org chart** (have a backing org Worker) are
  filtered out.

So the previous interpretation (filter by `global` / `owner_type === 'system'`) is wrong and
is dropped. The correct signal is **org-chart Worker membership**.

## User Stories

### US-1: Live-switch only between standalone external agents (primary)
As a user viewing a running spec task session, I want the top live-switch dropdown to list
only external agents that are NOT part of the Helix Org chart, so I don't switch the session
to an org-chart Worker agent or a non-external agent.

**Acceptance criteria**
- The `SwitchAgentControl` dropdown shows only `zed_external` agents.
- Agents that back a Helix Org-chart Worker are excluded.
- Standalone external agents — including those owned by a customer org
  (`owner_type === 'org'`) — remain shown.

### US-2: Same filtering in the details-panel selector (secondary)
As a user, I want the details-panel agent selector to apply the same filtering so the two
dropdowns are consistent (only external, non-org-chart agents; non-external no longer listed).

### US-3: Don't break the active/assigned agent
As a user, the currently active agent (live-switch: `session.parent_app`; details panel:
task `helix_app_id`) must always remain visible and selected, even if it would otherwise be
filtered out, so neither dropdown shows a blank/invalid selection.

### US-4: Sensible empty state
If no eligible agents exist, both dropdowns behave gracefully (no crash, no broken
selection), as they do today with an empty list.

### US-5: No regression when the feature is off
When `HELIX_ORG_ENABLED` is false (no org-chart Workers exist), behaviour for the external
filter is unchanged — there is simply nothing extra to exclude.

## Out of Scope
- No changes to the org-chart feature itself, the in-place switch mechanism, or how agents
  are created/owned.
- Other agent pickers (NewSpecTaskForm, ProjectSettings, AgentSelectionModal, etc.) are not
  changed unless they share the exact same code path.
