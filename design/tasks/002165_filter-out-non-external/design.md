# Design: Filter Helix-Org-Chart and Non-External Agents from Spec Task Agent Switchers

## Summary

Two parts:
- **Frontend (cheap):** keep only `zed_external` agents in both spec-task dropdowns.
- **Backend (required):** expose a per-app marker that says "this app backs a Helix
  Org-chart Worker", because `GET /api/v1/apps` currently has **no way** for the frontend to
  tell. The frontend then filters those out.

This stopped being frontend-only once we learned the real meaning of "Helix Org" (Phil's
org-chart feature). The link between an org-chart Worker and its App lives only in the
org sidecar tables and is not surfaced on the App.

## Key facts from codebase research

- **Two dropdowns, one source.** `SwitchAgentControl`
  (`frontend/src/components/session/SwitchAgentControl.tsx`, `eligibleAgents` ~lines 64–71)
  and the details-panel `AgentDropdown` (`SpecTaskDetailContent.tsx`, `sortedApps`
  ~lines 248–263) both read `useApps()` → `GET /api/v1/apps`.
- **External marker.** `AGENT_TYPE_ZED_EXTERNAL = 'zed_external'`
  (`frontend/src/types.ts:86`), checked on `config.helix.assistants[].agent_type` and/or
  `config.helix.default_agent_type`.
- **Org-chart link.** An AI Worker's App id is stored in `org_worker_runtime_state`
  (backend `helix`, key `agent_app_id`), keyed by `org_id` + `worker_id`. See
  `api/pkg/org/infrastructure/runtime/helix/state.go` (`WorkerState.AgentAppID`) and
  `api/pkg/org/infrastructure/persistence/gorm/worker_runtime.go`. Workers are listed by
  `GET /api/v1/orgs/{org}/workers` (`api/pkg/org/interfaces/server/api/workers.go`,
  `WorkerDTO.agentAppId`).
- **The App has no org-chart field.** `types.App` / `IApp` expose only `organization_id`,
  `owner`, `owner_type`, `global` — none of which distinguish an org-chart Worker app.
  `GET /api/v1/apps` returns org-chart Worker apps mixed in with normal apps.
- **Feature gate.** The org-chart feature is behind `HELIX_ORG_ENABLED` (default false). When
  off, there are no Workers, so the new marker is simply always false.

## Decision: backend computes an `is_helix_org_agent` flag on the apps listing

Add a **non-persisted, computed** boolean to the App returned by `GET /api/v1/apps`:

- Backend (`api/pkg/types/types.go`): add `IsHelixOrgAgent bool` with
  `json:"is_helix_org_agent" gorm:"-"` to `App` (computed, not stored).
- In the apps list handler (`api/pkg/server/app_handlers.go`, `listApps` /
  `listOrganizationApps`): when `HELIX_ORG_ENABLED` is on and an org is in scope, gather the
  set of `agent_app_id`s from `org_worker_runtime_state` for that org and set
  `IsHelixOrgAgent = true` on matching apps. When the feature is off, leave it false (no
  extra query). Use the existing org store/repository for the lookup; do not add a new table.
- Frontend (`frontend/src/types.ts`): add `is_helix_org_agent?: boolean` to `IApp`.

**Why a computed flag (not a client-side worker query):** keeps the dropdown logic simple
and synchronous, avoids a second round-trip and loading/race handling in the dropdowns,
keeps the org-chart knowledge in the backend (single source of truth), and naturally
no-ops when the feature is off.

*Alternative considered:* have the frontend call `GET /api/v1/orgs/{org}/workers`, build a
`Set` of `agentAppId`, and filter client-side. Rejected as primary because it couples both
dropdowns to the helix-org service, adds a query + loading state, and only works in org
context. Acceptable fallback if a backend change is undesirable.

## Frontend filtering (both dropdowns)

Shared predicate helpers (place where both components import them):

```ts
export const isExternalAgent = (app: IApp) =>
  app.config?.helix?.assistants?.some(a => a.agent_type === AGENT_TYPE_ZED_EXTERNAL) ||
  app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL;

export const isHelixOrgChartAgent = (app: IApp) => app.is_helix_org_agent === true;

export const isSpecTaskSwitchableAgent = (app: IApp) =>
  isExternalAgent(app) && !isHelixOrgChartAgent(app);
```

- **SwitchAgentControl:** replace the `eligibleAgents` filter with
  `apps.apps.filter(isSpecTaskSwitchableAgent)`. The current agent (`session.parent_app`)
  already renders as the dropdown value even if not in the list; include it explicitly as a
  row if it should be selectable.
- **Details panel:** replace `sortedApps` with an `eligibleApps` memo using
  `isSpecTaskSwitchableAgent`, and re-add the currently-assigned agent
  (`selectedAgent` / `helix_app_id`) if it was filtered out (US-3):
  ```ts
  const eligibleApps = useMemo(() => {
    const list = (apps.apps ?? []).filter(isSpecTaskSwitchableAgent);
    if (selectedAgent && !list.some(a => a.id === selectedAgent)) {
      const current = apps.apps?.find(a => a.id === selectedAgent);
      if (current) list.unshift(current);
    }
    return list;
  }, [apps.apps, selectedAgent]);
  ```
  Then point `<AgentDropdown agents={...} />` at `eligibleApps` and remove `sortedApps`.

## Edge cases & rationale

- **Customer-org agents stay.** We filter on org-chart Worker membership, not on
  `owner_type === 'org'`, so legitimately org-owned spec-task agents remain visible (US-1).
- **Active/assigned agent excluded by filter** (US-3): keep it visible so neither dropdown
  shows a blank selection.
- **Feature off** (US-5): no Workers → flag always false → only the external filter applies →
  no regression.
- **Empty result** (US-4): `AgentDropdown` already tolerates an empty `agents` array.

## Testing

- Backend: with `HELIX_ORG_ENABLED` on, an app backing an org-chart Worker is returned with
  `is_helix_org_agent: true`; a normal app returns false; with the feature off, all false.
- Frontend unit test for `isSpecTaskSwitchableAgent`: external + not org-chart → keep;
  external + org-chart → drop; non-external → drop.
- Manual: open a spec task with a running session in an org that has org-chart Workers.
  Confirm both dropdowns hide org-chart Worker agents and non-external agents, keep standalone
  external agents (including customer-org-owned), keep the active/assigned agent selected, and
  that in-place switching / `helix_app_id` updates still work.
