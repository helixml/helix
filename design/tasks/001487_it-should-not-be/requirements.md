# Requirements: Prevent Personal Agents on Organization Projects

## Problem

Users can attach a personal agent (app with no `organization_id`) to an organization project. This causes mysterious downstream failures because the agent's provider/config scope doesn't match the org's scope. Personal agents are a leftover from the removed "personal org" feature and should not appear in org project contexts at all.

## User Stories

1. **As an org member**, I should only see org-scoped agents when configuring a project's default agent, project manager agent, or PR reviewer agent — never personal agents.
2. **As an org member**, I should not be able to attach a personal agent to an org project via the API either — the backend should reject it.
3. **As a user who previously attached a personal agent to an org project**, I should see a clear indication that the agent is invalid so I can pick a valid one, rather than experiencing silent failures.

## Acceptance Criteria

### Frontend — Agent Dropdowns

- [ ] The `AgentDropdown` in `ProjectSettings.tsx` only shows agents belonging to the project's organization.
- [ ] The `AgentDropdown` in `SpecTaskDetailContent.tsx` only shows agents belonging to the task's project organization.
- [ ] Personal agents (those with empty `organization_id`) are never shown in any agent dropdown within an org project context.
- [ ] If a project currently references a personal agent, the dropdown shows a warning (e.g. "Invalid agent — not in this organization") rather than silently selecting nothing.

### Backend — Validation

- [ ] `updateProject` rejects setting `default_helix_app_id`, `project_manager_helix_app_id`, or `pull_request_reviewer_helix_app_id` to an app whose `organization_id` doesn't match the project's `organization_id`. Returns HTTP 400 with a clear error message.
- [ ] `createProject` applies the same validation for `default_helix_app_id`.
- [ ] `createSpecTask` and `updateSpecTask` validate that `helix_app_id` (if set) belongs to the same org as the parent project.

### Out of Scope

- Migrating existing broken project→agent associations (users can fix these manually once they see the warning).
- Removing the concept of personal apps entirely from the data model.
- Changes to the `listApps` API endpoint filtering (it already correctly filters by `organization_id` when the param is provided).