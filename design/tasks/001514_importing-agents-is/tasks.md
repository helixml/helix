# Implementation Tasks

## Route Migration

- [x] ~~In `frontend/src/router.tsx`, add new org-scoped route~~ — **Not needed.** External links can't include an org ID, so the route stays at `/import-agent`. Instead, the component resolves the org internally.
- [x] ~~In `frontend/src/router.tsx`, convert the existing `import-agent` route into a redirect~~ — **Not needed.** Same reason as above.

## Fix Organization ID in Import Payload

- [x] In `frontend/src/pages/ImportAgent.tsx`, add org resolution logic: read user's orgs from `account.organizationTools.organizations`, default to the org stored in `localStorage("selected_org")`, and add a dropdown for users with multiple orgs
- [x] Change `organization_id` in `handleImport()` to use `selectedOrgId` from the dropdown state — removes the `configData.organization_id` fallback so the exporter's org is never used
- [x] Pass `org_id` explicitly in all `orgNavigate()` calls using the selected org's name, since this page has no `:org_id` route param

## Testing

- [x] Test: visit `/import-agent?config=...` while logged in with an org → agent preview renders correctly
- [x] Test: click Import → agent created under correct org → navigates to `/orgs/{org}/app/{app_id}` without crash
- [x] Test: visit import URL while not logged in → shows login prompt → after login, returns to import page with config preserved
- [x] `cd frontend && yarn build` passes without errors