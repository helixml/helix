# Implementation Tasks

## Route Migration

- [~] In `frontend/src/router.tsx`, add new org-scoped route: `{ name: 'org_import-agent', path: '/orgs/:org_id/import-agent', ... }` rendering `<ImportAgent />`
- [~] In `frontend/src/router.tsx`, convert the existing `import-agent` route into a redirect: read `selected_org` from `localStorage`, redirect to `/orgs/{org}/import-agent` preserving the `?config=...` query string. If no org stored, redirect to `/orgs`

## Fix Organization ID in Import Payload

- [ ] In `frontend/src/pages/ImportAgent.tsx` `handleImport()` (L491), change `organization_id` to `account.organizationTools.organization?.id || ''` — remove the `configData.organization_id` fallback so the exporter's org is never used

## Testing

- [ ] Test: visit old-format URL `/import-agent?config=...` while logged in with an org selected → should redirect to `/orgs/{org}/import-agent?config=...`
- [ ] Test: visit old-format URL `/import-agent?config=...` with no stored org → should redirect to `/orgs`
- [ ] Test: visit new-format URL `/orgs/{org}/import-agent?config=...` → agent preview renders, org name visible in sidebar/breadcrumb
- [ ] Test: click Import → agent created under correct org → navigates to `/orgs/{org}/app/{app_id}` without crash
- [ ] Test: visit import URL while not logged in → shows login prompt → after login, returns to import page with config preserved
- [ ] `cd frontend && yarn build` passes without errors