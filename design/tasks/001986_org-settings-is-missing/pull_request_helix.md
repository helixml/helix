# Add Providers link to org sidebar

## Summary

The Org sidebar lists People, Teams, Billing, API Keys, and Settings — but had no link to `/orgs/:org_id/providers`, even though the route and `Providers` page were already wired. Org admins had to type the URL by hand to configure org-scoped LLM providers. This adds the missing sidebar item.

## Changes

- `frontend/src/components/orgs/OrgSidebar.tsx`: import `Plug` from `lucide-react` and insert a "Providers" item between "API Keys" and "Settings" that navigates to the existing `org_providers` route. No new files, no router changes, no backend changes.

## Why this placement / icon

- **Between API Keys and Settings**: both Providers and API Keys are integration / credential surfaces, so they cluster naturally; Settings stays last as the catch-all item.
- **`Plug` icon**: already used in `GitRepoDetail.tsx` and `CodeIntelligenceTab.tsx` for connector / "Connect" UI in this codebase, so it's the consistent semantic choice for "Providers".

## Testing

- `yarn build` (run inside `helix-frontend-1`) passes with no TypeScript errors.
- Verified end-to-end in the browser: registered, created an org, opened `/orgs/test-org/people`, confirmed the "Providers" item appears in the sidebar, clicked it, confirmed navigation to `/orgs/test-org/providers` and that the Providers page renders correctly.

## Screenshots

![Org sidebar — Providers link added between API Keys and Settings](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001986_org-settings-is-missing/screenshots/01-org-sidebar-with-providers.png)

![Providers page after clicking the new sidebar link](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001986_org-settings-is-missing/screenshots/02-providers-page-after-click.png)
