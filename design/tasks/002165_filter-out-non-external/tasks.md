# Implementation Tasks: Filter Spec Task Agent Switcher to External, Spec-Task-Owned Agents

- [ ] In `frontend/src/components/tasks/SpecTaskDetailContent.tsx`, add an `isExternal(app)` helper that returns true when any assistant has `agent_type === AGENT_TYPE_ZED_EXTERNAL` or `config.helix.default_agent_type === AGENT_TYPE_ZED_EXTERNAL`.
- [ ] Add an `isHelixOrgOwned(app)` helper that returns true when `app.global === true` or `app.owner_type === 'system'`.
- [ ] Replace the `sortedApps` memo with an `eligibleApps` memo that filters `apps.apps` to `isExternal(app) && !isHelixOrgOwned(app)`.
- [ ] In the same memo, ensure the currently-assigned agent (`selectedAgent` / `helix_app_id`) is included even if it would be filtered out, so the dropdown selection stays valid.
- [ ] Update the `<AgentDropdown agents={...} />` prop to use the new filtered list and remove the now-unused `sortedApps` code.
- [ ] Verify the empty-list case renders without errors (no eligible agents).
- [ ] Manually verify on the spec task details page: only external, non-Helix-org agents appear; the previously-assigned agent stays visible/selected; switching agents still persists `helix_app_id`.
- [ ] Update/add a small test for the filter predicates if a test for the old `sortedApps` exists.
