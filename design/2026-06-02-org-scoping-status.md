# helix-org multi-tenant refactor — status update

Branch: `refactor/helix-org-redesign`

## Verified end-to-end (localhost:8080)

- **Two orgs, isolated charts** ✅
  - `/orgs/test/helix-org/chart` and `/orgs/beta/helix-org/chart` each show their own `w-owner` / `p-root` / `r-owner`.
  - `w-alice` hired in `test` does not appear in `beta`.
- **Same worker ID across tenants** ✅
  - Composite (id, org_id) PK accepts two `w-alice` rows (one per org).
- **FK cascade on org delete** ✅
  - `DELETE FROM organizations WHERE id='org_…';` cascade-deletes every `org_*` row for that org. Verified org_workers/org_streams/org_positions tables all clear for the deleted org, untouched for others.
- **Browser flow** ✅
  - Chart, Workers list, Worker detail, Settings, Streams pages all render at `/orgs/<slug>/helix-org/...`. SSE on streams page shows "connected".
- **Lazy bootstrap** ✅
  - First request for a new org triggers `bootstrap.Run` with that org's ID; subsequent requests fast-path.
- **Per-org service api_key** ✅
  - `ensureHelixOrgServiceAPIKey` provisions a fresh key into the org's config registry on first bootstrap.
- **Per-org envs dir** ✅
  - Workers' envs land under `/filestore/helix-org/envs/<orgID>/w-owner/`.

## Architecture decisions baked in

1. Composite (id, org_id) PKs on every `org_*` table.
2. FK `org_id` → `organizations(id) ON DELETE CASCADE`.
3. Domain constructors require orgID at New() time.
4. URL surface: `/api/v1/orgs/{org}/helix-org/*`; React routes at `/orgs/:org_id/helix-org/{chart,workers,settings,streams}`.
5. Bootstrap is per-org, lazy on first request via `helix_org_middleware.go`.
6. Stream `idx_stream_org_name` unique constraint is composite (org_id, name).
7. Per-org MCP backend: gateway URL `/api/v1/mcp/helix-org/{org}/workers/{id}/mcp` resolves the org via the URL prefix.

## Remaining

- **Tests** — sub-agent is bulk-fixing test signatures (orgID arg insertion).  Production code already compiles green.
- Frontend nav/breadcrumb polish: link "Helix Org" entry in sidebar still navigates correctly via `currentOrgSlug`.

