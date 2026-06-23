# Implementation Tasks: Filter Spec Task Agent Switchers to External, Spec-Task-Owned Agents

- [ ] Add shared predicate helpers — `isExternalAgent(app)` (assistant `agent_type === AGENT_TYPE_ZED_EXTERNAL` OR `config.helix.default_agent_type === AGENT_TYPE_ZED_EXTERNAL`), `isHelixOrgAgent(app)` (`app.global === true` OR `app.owner_type === 'system'`), and `isSpecTaskSwitchableAgent(app)` (external AND not Helix-org). Place where both components can import them.
- [ ] Update `frontend/src/components/session/SwitchAgentControl.tsx`: replace the `eligibleAgents` filter with `apps.apps.filter(isSpecTaskSwitchableAgent)`.
- [ ] In `SwitchAgentControl`, ensure the currently active agent (`session.parent_app`) stays visible/selected even if it would be filtered out.
- [ ] Update `frontend/src/components/tasks/SpecTaskDetailContent.tsx`: replace the `sortedApps` memo with an `eligibleApps` memo that filters via `isSpecTaskSwitchableAgent` and re-adds the currently-assigned agent (`selectedAgent` / `helix_app_id`) if it was filtered out.
- [ ] Point the details-panel `<AgentDropdown agents={...} />` at the new `eligibleApps` and remove the unused `sortedApps` code.
- [ ] Verify the empty-list case renders without errors in both dropdowns.
- [ ] Manually verify on the spec task details page: top live-switch dropdown and details-panel dropdown both show only external, non-Helix-org agents; an active/assigned filtered agent still shows and stays selected; in-place switching and `helix_app_id` updates still work.
- [ ] Add a unit test for `isSpecTaskSwitchableAgent` (external user-owned → keep; external global/system → drop; non-external → drop).
