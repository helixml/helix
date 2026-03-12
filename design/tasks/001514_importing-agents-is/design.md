# Design: Fix Agent Import (Org-Awareness)

## Root Cause

The `/import-agent` route was never migrated to the org-scoped routing scheme. All other routes moved under `/orgs/:org_id/...` but this one was left behind at a top-level path. Without `:org_id` in the route, `useOrganizations` has no `router.params.org_id` to read, so `organization` stays `undefined`. Post-import navigation via `orgNavigate` then crashes because it can't build the org-scoped URL.

## Codebase Context

Key files and patterns discovered during investigation:

| File | Role |
|------|------|
| `frontend/src/pages/ImportAgent.tsx` | The import page component. Parses gzipped+base64 config from `?config=` query param, shows preview, calls `POST /api/v1/apps` |
| `frontend/src/router.tsx` | Route definitions. All main routes are `org_*` under `/orgs/:org_id/...`. Import route is orphaned at `/import-agent` |
| `frontend/src/hooks/useOrganizations.ts` | Reads `router.params.org_id` (L99) to determine current org. No param → no org |
| `frontend/src/contexts/account.tsx` | `orgNavigate()` (L391) prepends `org_` to route name and injects `org_id` from `organizationToolsRef.current.organization?.name` |
| `api/pkg/server/app_handlers.go` | `createApp()` (L374) — backend is already org-aware, accepts `organization_id` in request body |

**Pattern**: Every org-scoped route follows the same structure: `name: 'org_<name>'`, `path: '/orgs/:org_id/<path>'`. The `useOrganizations` hook auto-loads the org from the URL param. `orgNavigate` auto-injects `org_id` when navigating.

## Solution

**Move the import-agent route under the org scope.** This is the simplest fix and follows the established pattern — no special-casing required.

### Route Change

```
// Before
{ name: 'import-agent', path: '/import-agent', ... }

// After
{ name: 'org_import-agent', path: '/orgs/:org_id/import-agent', ... }
```

Once the route has `:org_id`, the existing `useOrganizations` hook will automatically resolve the org, and `orgNavigate` will work because `organization.name` will be populated.

### Backward Compatibility (redirect)

External links (e.g. from Launchpad at `app.helix.ml`) use the old `/import-agent?config=...` URL. We need a redirect route that:

1. Keeps the old `/import-agent` path as a lightweight redirect route
2. Reads the user's current/default org (from `localStorage` key `selected_org`, same as the root `/` redirect logic in `router.tsx`)
3. Redirects to `/orgs/{org}/import-agent?config=...` preserving the query string

If no org is stored (fresh user), redirect to `/orgs` — the normal onboarding flow will handle org creation, and the user can retry the link.

### Fix `organization_id` in Import Payload

In `ImportAgent.tsx` `handleImport()` (L491), the `organization_id` currently falls back to `configData.organization_id` (the exporter's org). Change to:

```
organization_id: account.organizationTools.organization?.id || ''
```

Drop the `configData.organization_id` fallback entirely. The org ID must always come from the importing user's context, never from the exported config.

### Navigation After Import

No changes needed — `navigateToApp` already calls `account.orgNavigate('app', { app_id })`, which will work correctly once the org is resolved from the route param.

## Alternatives Considered

1. **Add an org picker to the import page** — Overengineered for this fix. The user already has an org selected via the sidebar. Can be added later if needed.
2. **Keep the route top-level and manually resolve the org** — Breaks the established pattern. Every other page uses the URL param; making import special would be confusing and fragile.
3. **Auto-select first org if none in URL** — Risky. Could import into the wrong org silently. Better to require explicit org context via the URL.

## Risk Assessment

- **Low risk**: The backend `POST /api/v1/apps` endpoint is unchanged. It already validates org membership.
- **Medium risk**: External links from Launchpad will hit the old URL. The redirect route handles this, but we should verify Launchpad doesn't hardcode assumptions about the URL structure.
- **Testing**: Navigate to an import URL → verify redirect → verify org displayed → import → verify navigation to agent editor works. Also test: not-logged-in flow, user with multiple orgs.