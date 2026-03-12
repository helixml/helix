# Requirements: Fix Agent Import (Org-Awareness)

## Problem

The `/import-agent` route is not org-aware. After the recent org migration (all routes now live under `/orgs/:org_id/...`), the import page still lives at a top-level path with no org context. This causes:

1. **Crash on navigation after import** — `orgNavigate('app', { app_id })` tries to build `/orgs/:org_id/app/:app_id` but `org_id` is `undefined` because `useOrganizations` reads it from `router.params.org_id`, which doesn't exist on the `/import-agent` route. Sentry error: `Cannot build path: '/orgs/:org_id/app/:app_id' requires missing parameters { org_id }`.
2. **Wrong or empty `organization_id` sent to API** — The import sends `account.organizationTools.organization?.id || configData.organization_id || ''`. With no org selected, the agent may be created with the wrong org (from the exporter's config) or no org at all.

## User Stories

1. **As a user clicking an import link**, I want to import an agent into my currently selected organization so it appears alongside my other agents.
2. **As a user with multiple orgs**, I want to choose which organization to import into (or have it default to my current org).
3. **As a user who is not logged in**, I want to log in and then be redirected back to complete the import.

## Acceptance Criteria

- [ ] Visiting an import-agent URL no longer crashes with "requires missing parameters { org_id }"
- [ ] The imported agent is created under the user's current organization
- [ ] After successful import, the user is navigated to the agent editor page (existing behavior, but working)
- [ ] The login-then-redirect flow still works (user sees "Sign In to Continue", logs in, returns to import)
- [ ] Users with multiple orgs can see which org the agent will be imported into
- [ ] The `organization_id` sent to `POST /api/v1/apps` is always the user's selected org, never the exporter's org from the config payload

## Out of Scope

- Org picker dropdown on the import page (nice-to-have, not required — defaulting to current org is sufficient)
- Changes to the export/launchpad side
- Changes to the `POST /api/v1/apps` backend endpoint (it already handles `organization_id` correctly)