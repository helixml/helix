# Requirements: Filter Spec Task Agent Switchers to External, Spec-Task-Owned Agents

## Background

The spec task details page (`SpecTaskDetailContent.tsx`) has **two** agent-picker
dropdowns, both built from the same apps context (`useApps()` → `GET /api/v1/apps`) and
both rendered with the shared `AgentDropdown` component:

1. **Live-switch control (primary target)** — `SwitchAgentControl`, mounted at the top of
   the chat panel header. This is the dropdown Kai recently added that switches the
   *running session* to a different agent **in place** (same session id, same container,
   transcript preserved). See `design/tasks/002111_so-we-recently-added-a/`.
   It already filters to `zed_external` agents but does **not** exclude Helix-org-owned
   (global / system) agents.
2. **Details-panel agent selector (secondary target)** — the `AgentDropdown` in the
   details sidebar that sets the task's assigned agent (`helix_app_id`). This one currently
   only *sorts* external agents to the top and still lists non-external agents.

We want both dropdowns to offer only agents that make sense for a spec task: **external
(`zed_external`) agents owned by the user/org**, excluding non-external agents and agents
owned by the Helix org (global / system-owned agents).

## User Stories

### US-1: Live-switch only between relevant agents (primary)
As a user viewing a running spec task session, I want the live-switch dropdown at the top
of the chat panel to list only external agents that belong to me/my org, so I don't
accidentally switch the session to a non-external or built-in Helix agent.

**Acceptance criteria**
- The `SwitchAgentControl` dropdown shows only `zed_external` agents.
- Agents owned by the Helix org (`global === true` or `owner_type === 'system'`) are excluded.
- User/org-owned agents (`owner_type` of `user` or `org`, `global === false`) remain shown.

### US-2: Same filtering in the details-panel selector (secondary)
As a user, I want the details-panel agent selector to apply the same filtering, so the two
dropdowns are consistent.

**Acceptance criteria**
- The details-panel `AgentDropdown` shows only external, non-Helix-org agents.
- Non-external agents are no longer listed (current behaviour merely sorts them lower).

### US-3: Don't break the active/assigned agent
As a user, I want the currently active agent (live-switch: `session.parent_app`;
details panel: task `helix_app_id`) to always remain visible and selected, even if it would
otherwise be filtered out, so neither dropdown shows a blank/invalid selection.

**Acceptance criteria**
- If the current agent would be excluded by the filter, it is still included so the
  dropdown value renders with its correct label.

### US-4: Sensible empty state
As a user, if no eligible agents exist, both dropdowns behave gracefully (no crash, no
broken selection), as they do today with an empty list.

## Out of Scope
- No backend/API changes. Filtering is client-side from apps already returned by
  `/api/v1/apps`.
- No changes to the in-place switch mechanism itself, or to how agents are created/owned.
- Other agent pickers (NewSpecTaskForm, ProjectSettings, AgentSelectionModal, etc.) are not
  changed unless they share the exact same code path.
