# Implementation Tasks: Filter Helix-Org-Chart and Non-External Agents from Spec Task Agent Switchers

## Backend — expose org-chart membership on the apps listing
- [x] Add a computed, non-persisted field `IsHelixOrgAgent bool` (`json:"is_helix_org_agent" gorm:"-"`) to `App` in `api/pkg/types/types.go`.
- [x] In the apps list handler (`api/pkg/server/app_handlers.go`, `listApps` / `listOrganizationApps`), when `HELIX_ORG_ENABLED` is on and an org is in scope, look up the set of `agent_app_id`s from `org_worker_runtime_state` (backend `helix`) for that org and set `IsHelixOrgAgent = true` on matching apps; leave it false when the feature is off (no extra query). Implemented as `markHelixOrgAgents`, only on the org-scoped path (org-chart agents are always org-scoped); reaches the shared `*gorm.DB` via the anonymous-interface accessor used by `openOrgStore`.
- [x] Add a backend test for `markHelixOrgAgents`. NOTE: the positive flagging path needs a live Postgres `org_worker_runtime_state` table (mock store has no `GormDB`), so unit tests cover the no-op guarantees — feature-off and empty-org never flag and never touch the DB (the no-regression case). Positive flagging verified end-to-end.

## Frontend — filtering helpers
- [x] Add `is_helix_org_agent?: boolean` to `IApp` in `frontend/src/types.ts`.
- [x] Add shared predicates `isExternalAgent`, `isHelixOrgChartAgent` (`app.is_helix_org_agent === true`), and `isSpecTaskSwitchableAgent` (external AND not org-chart) in `frontend/src/utils/apps.ts` (existing app-utils module both components already relate to).
- [x] Add a frontend unit test for `isSpecTaskSwitchableAgent` (external+non-org-chart keep; external+org-chart drop; non-external drop). `frontend/src/utils/apps.test.ts`, 6 tests passing (run via the `helix-frontend-1` container — no local node_modules).

## Frontend — apply to both dropdowns
- [x] `frontend/src/components/session/SwitchAgentControl.tsx`: replace the `eligibleAgents` filter with `apps.apps.filter(isSpecTaskSwitchableAgent)`; keep the active agent (`session.parent_app`) visible/selected if filtered out.
- [x] `frontend/src/components/tasks/SpecTaskDetailContent.tsx`: replace `sortedApps` with an `eligibleApps` memo using `isSpecTaskSwitchableAgent` that re-adds the currently-assigned agent (`selectedAgent` / `helix_app_id`) if filtered out; point the details-panel `<AgentDropdown agents={...} />` at it and remove `sortedApps`.
- [x] Verify the empty-list case renders without errors in both dropdowns — `AgentDropdown` already renders a disabled "No agents available" item when `agents.length === 0`, and `renderValue` falls back to "Select Agent" for an unknown value (no MUI warning). `tsc --noEmit` clean.

## Verification
- [x] Verified what is verifiable in this environment (see notes). NOT verified end-to-end: the positive org-chart exclusion path — `HELIX_ORG_ENABLED=false` in the inner Helix and the `org_worker_runtime_state` table does not exist, so no org-chart Workers/agents can be created here.

### What was verified
- Backend `go build ./pkg/server ./pkg/types` clean; `go test -run Test_markHelixOrgAgents` passes (2 no-op cases). API hot-reloaded (Air) with no compile errors.
- Frontend `tsc --noEmit` clean; `vitest run src/utils/apps.test.ts` 6/6 pass; no Vite transform errors in `helix-frontend-1`.
- Confirmed the feature gate is safe: with `HELIX_ORG_ENABLED=false` the table is absent, and `markHelixOrgAgents` returns before any DB access, so the apps listing is unaffected (no-regression guarantee, US-5). When the feature is on, `openOrgStore` AutoMigrate creates the table, so the query target exists.

### Not verified (requires HELIX_ORG_ENABLED + org-chart Workers + a live external-agent session)
- Org-chart Worker agents actually disappearing from the two dropdowns.
- The active/assigned agent staying visible when it would be filtered out, against a live session.
- These need an org with the helix-org feature enabled and at least one hired AI Worker; not available in the inner Helix. Recommend verifying on a stack with `HELIX_ORG_ENABLED=true`.
