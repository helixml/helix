# Design: Filter Spec Task Agent Switchers to External, Spec-Task-Owned Agents

## Summary

A **frontend-only** change touching two dropdowns that share the same apps source and the
same `AgentDropdown` component. The primary target is the **live-switch control**
(`SwitchAgentControl`); the same filtering is also applied to the **details-panel selector**.

## Key Files (from codebase research)

- `frontend/src/components/session/SwitchAgentControl.tsx` — **primary**.
  - `eligibleAgents` memo (~lines 64–71) currently filters `apps.apps` to apps where an
    assistant has `agent_type === AGENT_TYPE_ZED_EXTERNAL`.
  - Current value comes from `session?.parent_app` (~line 76); the dropdown can show any id
    as its value, so the active agent stays visible even if not in the list.
  - Rendered at the top of the chat panel header in `SpecTaskDetailContent.tsx` (~lines 1999–2007).
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — **secondary**.
  - `sortedApps` memo (~lines 248–263) currently *sorts* external first but keeps everything.
  - Details-panel `<AgentDropdown ... agents={sortedApps} />` (~lines 1451–1461); current
    value is `selectedAgent` (task `helix_app_id`).
- `frontend/src/components/agent/AgentDropdown.tsx` — shared presentational `Select`; no change.
- `frontend/src/contexts/apps.tsx` — apps source (`GET /api/v1/apps`); no change.
- `frontend/src/types.ts`
  - `AGENT_TYPE_ZED_EXTERNAL = 'zed_external'` (line 86).
  - `IApp`: `global: boolean`, `owner_type: IOwnerType` (`'user' | 'system' | 'org'`),
    `config.helix.assistants[].agent_type`, `config.helix.default_agent_type`.

## How to identify each category

**External agent** (keep): an app where any assistant has
`agent_type === AGENT_TYPE_ZED_EXTERNAL`, OR `config.helix.default_agent_type === AGENT_TYPE_ZED_EXTERNAL`.
- `SwitchAgentControl` today checks only the assistants array; the details panel also checks
  `default_agent_type`. We keep the **more inclusive** predicate (assistants OR
  `default_agent_type`) and use the same one in both places for consistency.

**Owned by Helix org** (exclude): there is **no hard-coded Helix org id/slug** in the
codebase (confirmed by research). The reliable signal for first-party / built-in agents,
already exposed on `IApp`, is `app.global === true` OR `app.owner_type === 'system'`.
User/org-created spec-task agents have `global === false` and `owner_type` of `'user'`/`'org'`.

## Decision: one shared filter helper, applied in both components

Add a small reusable predicate (e.g. in a shared util or co-located and used by both) to
avoid divergence:

```ts
export const isExternalAgent = (app: IApp) =>
  app.config?.helix?.assistants?.some(a => a.agent_type === AGENT_TYPE_ZED_EXTERNAL) ||
  app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL;

export const isHelixOrgAgent = (app: IApp) =>
  app.global === true || app.owner_type === 'system';

export const isSpecTaskSwitchableAgent = (app: IApp) =>
  isExternalAgent(app) && !isHelixOrgAgent(app);
```

**SwitchAgentControl** — replace the `eligibleAgents` filter with
`apps.apps.filter(isSpecTaskSwitchableAgent)`. The current agent (`session.parent_app`)
already stays visible as the dropdown value, so no extra re-add step is strictly required,
but if the active agent should appear as a selectable row, include it explicitly when missing.

**Details panel** — replace `sortedApps` with an `eligibleApps` memo:
```ts
const eligibleApps = useMemo(() => {
  const list = (apps.apps ?? []).filter(isSpecTaskSwitchableAgent);
  if (selectedAgent && !list.some(a => a.id === selectedAgent)) {
    const current = apps.apps?.find(a => a.id === selectedAgent);
    if (current) list.unshift(current); // keep current selection valid (US-3)
  }
  return list;
}, [apps.apps, selectedAgent]);
```
Pass `eligibleApps` as the `agents` prop and remove the old `sortedApps` logic.

## Edge cases & rationale

- **Active/assigned agent excluded by filter** (US-3) — keep it visible so neither dropdown
  renders an empty/invalid selection. `SwitchAgentControl` already tolerates this via its
  value handling; the details panel needs the explicit re-add shown above.
- **No hard-coded Helix org** — `global`/`owner_type === 'system'` is the closest correct
  signal and is already on `IApp`. The `isHelixOrgAgent` predicate is isolated so it can be
  swapped for an explicit Helix org id later if product defines one.
- **Empty result** (US-4) — `AgentDropdown` already tolerates an empty `agents` array.

## Testing

- Manual: open a spec task with a running session. Confirm the top live-switch dropdown
  lists only external, non-global/non-system agents; confirm the details-panel dropdown
  matches; confirm an active/assigned agent that would be filtered still shows and stays
  selected; confirm switching still works (in-place switch persists / `helix_app_id` updates).
- Add a small unit test for the `isSpecTaskSwitchableAgent` predicate covering: external
  user-owned (keep), external global/system (drop), non-external (drop).
