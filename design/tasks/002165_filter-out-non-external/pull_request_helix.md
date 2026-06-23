# Filter org-chart and non-external agents from spec task agent switchers

## Summary

The spec task details page has two agent pickers — the live-switch control at the top of
the chat panel (`SwitchAgentControl`) and the details-panel selector (`AgentDropdown`). Both
listed agents that don't make sense for a spec task: non-external agents, and agents that
belong to the **Helix org chart** (Phil's org-chart-of-agents feature, `HELIX_ORG_ENABLED`).

This change limits both dropdowns to standalone external (`zed_external`) agents and hides
org-chart Worker agents. Agents owned by a customer organization are unaffected — only
org-chart Worker agents are excluded.

## Changes

**Backend**
- Added computed, non-persisted `App.IsHelixOrgAgent` (`json:"is_helix_org_agent"`,
  `gorm:"-"`).
- `markHelixOrgAgents` in `app_handlers.go` flags apps that back a Helix org-chart Worker by
  looking up `org_worker_runtime_state` (`backend='helix'`, `key='agent_app_id'`) for the
  org. Wired into the org-scoped apps listing only (org-chart agents are always org-scoped).
  Gated on `HELIX_ORG_ENABLED` — a complete no-op (no DB access) when the feature is off.
- Reaches the shared `*gorm.DB` via the same anonymous-interface accessor used by
  `openOrgStore`, so it adds no hard dependency on the concrete store type.
- Unit tests for the feature-off / no-org no-op paths.

**Frontend**
- Added `is_helix_org_agent?` to `IApp`.
- New shared predicates in `utils/apps.ts`: `isExternalAgent`, `isHelixOrgChartAgent`,
  `isSpecTaskSwitchableAgent` (+ unit tests).
- `SwitchAgentControl` and the details-panel selector both filter with
  `isSpecTaskSwitchableAgent`, and keep the currently active/assigned agent visible even if
  it would be filtered out.

## Testing

- `go build` clean; backend unit tests pass (gate no-op cases).
- Frontend `tsc --noEmit` clean; `vitest` predicate tests 6/6 pass.
- NOT verified end-to-end: org-chart Worker agents disappearing from the dropdowns requires
  `HELIX_ORG_ENABLED=true` and a hired AI Worker — the inner Helix runs with the feature off
  and the `org_worker_runtime_state` table absent. Verify on a feature-enabled stack.
