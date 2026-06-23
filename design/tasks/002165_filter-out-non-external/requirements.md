# Requirements: Filter Spec Task Agent Switcher to External, Spec-Task-Owned Agents

## Background

The spec task details page (`SpecTaskDetailContent.tsx`) shows an **agent switcher** dropdown
(`AgentDropdown`) that lets the user change which agent is assigned to the task
(persisted as `helix_app_id`). Today that dropdown lists **every** app returned by the
apps context for the current org — it merely sorts external agents to the top but still
shows non-external agents and first-party/global Helix-org agents that are not meaningful
choices for a spec task.

We want the dropdown to only offer agents that make sense for spec tasks: **external
(`zed_external`) agents that belong to the user/org (spec-task agents)**, excluding
non-external agents and agents owned by the Helix org (global / system-owned agents).

## User Stories

### US-1: Only switch between relevant agents
As a user on the spec task details page, I want the agent switcher to list only the
external agents I can actually use for this spec task, so I'm not distracted by
non-external agents or built-in Helix agents that don't apply.

**Acceptance criteria**
- The agent dropdown shows only agents whose `agent_type` (any assistant) or
  `default_agent_type` is `zed_external`.
- Non-external agents are not shown.
- Agents owned by the Helix org (i.e. `global === true` or `owner_type === 'system'`)
  are not shown.
- Agents owned by the current user/org (`owner_type` of `user` or `org`, `global === false`)
  remain shown.

### US-2: Don't break the current selection
As a user, I want the agent currently assigned to the task to always remain visible and
selected in the dropdown, even if it would otherwise be filtered out, so the UI never
shows a blank/invalid selection.

**Acceptance criteria**
- If the task's currently assigned agent (`helix_app_id`) would be excluded by the filter,
  it is still included in the list so the selection renders correctly.

### US-3: Sensible empty state
As a user, if no eligible agents exist, the dropdown behaves gracefully (no crash, no
broken selection), consistent with how an empty agent list is handled today.

**Acceptance criteria**
- An empty filtered list does not error; the dropdown renders empty/disabled as it does
  today when there are no apps.

## Out of Scope
- No backend/API changes. Filtering is done client-side from the apps already returned by
  `/api/v1/apps`.
- No changes to how agents are created, owned, or assigned.
- No changes to other agent selectors (e.g. NewSpecTaskForm, ProjectSettings) unless they
  share the exact same code path.
