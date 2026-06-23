# Implementation Tasks: Filter Helix-Org-Chart and Non-External Agents from Spec Task Agent Switchers

## Backend — expose org-chart membership on the apps listing
- [~] Add a computed, non-persisted field `IsHelixOrgAgent bool` (`json:"is_helix_org_agent" gorm:"-"`) to `App` in `api/pkg/types/types.go`.
- [ ] In the apps list handler (`api/pkg/server/app_handlers.go`, `listApps` / `listOrganizationApps`), when `HELIX_ORG_ENABLED` is on and an org is in scope, look up the set of `agent_app_id`s from `org_worker_runtime_state` (backend `helix`) for that org and set `IsHelixOrgAgent = true` on matching apps; leave it false when the feature is off (no extra query).
- [ ] Add a backend test: org-chart Worker app → `is_helix_org_agent: true`; normal app → false; feature off → all false.

## Frontend — filtering helpers
- [ ] Add `is_helix_org_agent?: boolean` to `IApp` in `frontend/src/types.ts`.
- [ ] Add shared predicates `isExternalAgent`, `isHelixOrgChartAgent` (`app.is_helix_org_agent === true`), and `isSpecTaskSwitchableAgent` (external AND not org-chart) where both components can import them.
- [ ] Add a frontend unit test for `isSpecTaskSwitchableAgent` (external+non-org-chart keep; external+org-chart drop; non-external drop).

## Frontend — apply to both dropdowns
- [ ] `frontend/src/components/session/SwitchAgentControl.tsx`: replace the `eligibleAgents` filter with `apps.apps.filter(isSpecTaskSwitchableAgent)`; keep the active agent (`session.parent_app`) visible/selected if filtered out.
- [ ] `frontend/src/components/tasks/SpecTaskDetailContent.tsx`: replace `sortedApps` with an `eligibleApps` memo using `isSpecTaskSwitchableAgent` that re-adds the currently-assigned agent (`selectedAgent` / `helix_app_id`) if filtered out; point the details-panel `<AgentDropdown agents={...} />` at it and remove `sortedApps`.
- [ ] Verify the empty-list case renders without errors in both dropdowns.

## Verification
- [ ] Manually verify in an org with org-chart Workers: both dropdowns hide org-chart Worker agents and non-external agents, keep standalone external agents (including customer-org-owned), keep the active/assigned agent selected, and in-place switching / `helix_app_id` updates still work.
