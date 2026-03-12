# Implementation Tasks

## Backend — Org validation helper

- [ ] Add `validateAppBelongsToOrg(ctx, appID, orgID string) error` helper in `api/pkg/server/project_handlers.go`. It calls `s.Store.GetApp()` and checks `app.OrganizationID == orgID`. Returns a clear error message on mismatch.

## Backend — Project create/update validation

- [ ] In `createProject` (`api/pkg/server/project_handlers.go` ~L257), after fetching the default app, validate that `defaultApp.OrganizationID == req.OrganizationID`. Return HTTP 400 on mismatch.
- [ ] In `updateProject` (~L498–510), validate `DefaultHelixAppID`, `ProjectManagerHelixAppID`, and `PullRequestReviewerHelixAppID` against `project.OrganizationID` when the field is non-empty. Return HTTP 400 on mismatch.

## Backend — Spec task create/update validation

- [ ] In spec task create handler, when `HelixAppID` is set, look up the parent project's `OrganizationID` and validate the app belongs to that org. Return HTTP 400 on mismatch.
- [ ] In spec task update handler, apply the same validation when `HelixAppID` is being changed.

## Backend — Unit tests

- [ ] Add test: `createProject` with a personal agent (empty `organization_id`) on an org project returns 400.
- [ ] Add test: `updateProject` setting `default_helix_app_id` to a personal agent returns 400.
- [ ] Add test: `updateProject` setting `default_helix_app_id` to an agent from a *different* org returns 400.
- [ ] Add test: happy path — setting an agent that belongs to the same org succeeds.

## Frontend — Filter agents in ProjectSettings

- [ ] In `ProjectSettings.tsx`, update the `sortedApps` memo to filter out apps whose `organization_id` doesn't match `project.organization_id` before sorting.
- [ ] If `selectedAgentId` is set but not found in `sortedApps`, show an inline `<Alert severity="warning">` saying the current agent is not available in this organization.

## Frontend — Filter agents in SpecTaskDetailContent

- [ ] In `SpecTaskDetailContent.tsx`, apply the same org-based filter in the `sortedApps` memo.
- [ ] Show the same mismatch warning if the task's current agent isn't in the filtered list.

## Verification

- [ ] `cd api && go build ./pkg/server/` compiles cleanly.
- [ ] `cd frontend && yarn build` compiles cleanly.
- [ ] Manual test: open an org project settings page, confirm only org agents appear in all three agent dropdowns.
- [ ] Manual test: attempt to set a personal agent via API (`curl -X PUT` with a personal app ID) and confirm 400 response.