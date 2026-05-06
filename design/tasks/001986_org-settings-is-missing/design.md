# Design

## Summary

Add one item to the existing `sections` array in `OrgSidebar.tsx`. No new files, no router changes, no backend changes.

## Findings From Codebase Exploration

- **Sidebar component**: `frontend/src/components/orgs/OrgSidebar.tsx` (whole file is 73 lines). Uses the shared `ContextSidebar` widget and a single `Organization Management` section with 5 items (People, Teams, Billing, API Keys, Settings).
- **Route already registered**: `org_providers` at `/orgs/:org_id/providers` in `frontend/src/router.tsx:181-188`, with `meta: { drawer: false }` (consistent with billing / api keys).
- **Page already exists**: `frontend/src/pages/Providers.tsx` — same component renders both the global and the org-scoped providers screen.
- **Navigation pattern**: `router.navigate(routeName, { org_id: orgId })` via `handleNavigationClick(routeName)`. No `<Link>` / `<a href>` (per `helix/CLAUDE.md` — react-router5).

## The Change

Insert one item into the items array in `OrgSidebar.tsx`. Suggested placement: between **API Keys** and **Settings**, since "Providers" is a configuration concern that pairs naturally with API Keys (both are integration/credentials surfaces). Settings stays last as the "catch-all" item.

```tsx
{
  id: 'providers',
  label: 'Providers',
  icon: <Plug size={20} />,
  isActive: currentRouteName === 'org_providers',
  onClick: () => handleNavigationClick('org_providers')
}
```

Add `Plug` to the existing lucide-react import on line 3.

## Decisions

### Icon: `Plug`
The lucide-react `Plug` icon is already used elsewhere in this codebase for provider/connector UI (`GitRepoDetail.tsx:50,876`, `CodeIntelligenceTab.tsx:33,1405` for the "Connect" tab). Reusing it keeps icon semantics consistent — "Providers" is conceptually a connection/integration surface. Alternatives considered: `Cpu`, `Server`, `Cloud` — all weaker semantic fits.

### Placement: between API Keys and Settings
Both Providers and API Keys deal with external integrations / credentials, so they cluster naturally. Keeping Settings last preserves the existing convention of generic settings being the trailing item.

### Reuse, not refactor
A dedicated `OrgProvidersSidebarItem` component or a config-driven sidebar would be over-engineering for a one-line addition. The existing inline-array pattern is already used for the other five items — match it.

## Files Touched

- `frontend/src/components/orgs/OrgSidebar.tsx` — one import addition, one items-array entry.

## Testing

- Visual: log in (test creds in `helix/CLAUDE.md`), open an org, confirm the new "Providers" item appears, click it, confirm navigation to `/orgs/:org_id/providers` and that the item shows as active on that route.
- Build: `cd frontend && yarn build` must pass with no TS errors.
- No new unit tests warranted — this is a single declarative entry mirroring five existing siblings; the routing and page are already exercised.

## Risks

Effectively none. Worst case is a typo in the route name (`org_providers`) — caught immediately on click.
