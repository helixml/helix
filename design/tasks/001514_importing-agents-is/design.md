# Design: Fix Agent Import (Org-Awareness)

## Root Cause

The `/import-agent` route is not org-scoped (no `:org_id` in the URL), so `useOrganizations` has no `router.params.org_id` to read, meaning `organization` stays `undefined`. This causes two failures:

1. **Crash on post-import navigation** — `orgNavigate('app', { app_id })` tries to build `/orgs/:org_id/app/:app_id` but `org_id` is `undefined`. Sentry error: `Cannot build path: '/orgs/:org_id/app/:app_id' requires missing parameters { org_id }`.
2. **Wrong `organization_id` in API call** — The import payload fell back to `configData.organization_id` (the exporter's org), which could create the agent in the wrong org or fail authorization.

## Codebase Context

| File | Role |
|------|------|
| `frontend/src/pages/ImportAgent.tsx` | Import page component. Parses gzipped+base64 config from `?config=` query param, shows preview, calls `POST /api/v1/apps` |
| `frontend/src/router.tsx` | Route definitions. All main routes are `org_*` under `/orgs/:org_id/...`. Import route is at `/import-agent` (no org param) |
| `frontend/src/hooks/useOrganizations.ts` | Reads `router.params.org_id` (L99) to determine current org. No param → no org |
| `frontend/src/contexts/account.tsx` | `orgNavigate()` (L391) prepends `org_` to route name and injects `org_id` from `organizationToolsRef.current.organization?.name` |
| `api/pkg/server/app_handlers.go` | `createApp()` (L374) — backend is already org-aware, accepts `organization_id` in request body |
| `frontend/src/utils/localStorage.ts` | Exports `SELECTED_ORG_STORAGE_KEY = 'selected_org'` used across the app |

**Pattern**: Every org-scoped route follows `name: 'org_<name>'`, `path: '/orgs/:org_id/<path>'`. The `useOrganizations` hook auto-loads the org from the URL param. `orgNavigate` auto-injects `org_id` when navigating.

## Solution: Resolve Org In-Component

**Key insight**: We cannot move the route to `/orgs/:org_id/import-agent` because external links (from Launchpad at `app.helix.ml`) cannot know which org the user wants to import into. The route must stay at `/import-agent`.

Instead, the `ImportAgent` component resolves the org internally:

### 1. Org Resolution (new state in ImportAgent)

- Read the user's org list from `account.organizationTools.organizations`
- Add `selectedOrgId` state, defaulting to the org stored in `localStorage(SELECTED_ORG_STORAGE_KEY)`
- Fallback order: stored org → first org where user is a member → first org in list
- Show an org dropdown when `organizations.length > 1` so the user can change it

### 2. Fix API Payload

Changed from:
```
organization_id: account.organizationTools.organization?.id || configData.organization_id || ''
```
To:
```
organization_id: selectedOrgId
```

The exporter's org ID is never used. The org always comes from the importing user's selection.

### 3. Fix Post-Import Navigation

Pass `org_id` explicitly in all `orgNavigate()` calls:
```
const orgSlug = selectedOrg?.name || selectedOrgId;
account.orgNavigate("app", { app_id: appId, org_id: orgSlug });
```

This works because `orgNavigate` checks `params.org_id` before falling back to `organization?.name`. Since we pass it explicitly, it doesn't need the route param.

## Alternatives Considered

1. **Move route to `/orgs/:org_id/import-agent` with redirect from old URL** — External links can't include an org ID, so this would require a redirect that reads from localStorage. Adds complexity with two routes for no UX benefit. Rejected.
2. **Add an org picker modal before import** — Overengineered. A simple dropdown on the existing page is sufficient.
3. **Auto-select first org without showing picker** — Risky for multi-org users. The dropdown (shown only for multi-org users) makes the choice explicit.

## Implementation Notes

- The org dropdown is only shown when `organizations.length > 1` — single-org users see no change
- The `useEffect` for org resolution has a guard `if (selectedOrgId || organizations.length === 0) return` to avoid re-running after user manually selects an org
- The `organizations` list is loaded by the account context during `loadStatus()`, so it's available by the time the import page renders (same as every other page)
- `selectedOrg` is derived via `useMemo` from `selectedOrgId` for getting the name/slug

## Testing Results

- Login flow works: unauthenticated user → login page → returns to `/import-agent?config=...` with config preserved
- Import succeeds: agent created under correct org, model substitution dialog shows when needed
- Navigation works: after import, navigates to `/orgs/{org-name}/app/{app-id}` — no crash
- `yarn build` passes