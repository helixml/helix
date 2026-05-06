# Requirements

## Problem

The Org area sidebar (rendered for `/orgs/:org_id/*` routes) lists People, Teams, Billing, API Keys, and Settings — but does **not** link to `/orgs/:slug/providers`. The Providers page exists and is fully wired in the router, but org admins have no in-UI way to discover or reach it. They must hand-type the URL.

## User Story

As an **org admin**, I want a **"Providers" link in the Org sidebar** so that I can configure org-scoped LLM providers without typing the URL by hand.

## Acceptance Criteria

1. When viewing any `/orgs/:org_id/*` route, the org sidebar shows a **"Providers"** item alongside People, Teams, Billing, API Keys, Settings.
2. Clicking the item navigates to `/orgs/:org_id/providers` via the existing `org_providers` named route.
3. The item's `isActive` state is true when the current route is `org_providers` (matching the styling of the other items).
4. The item uses a lucide-react icon consistent with the rest of the sidebar (size 20).
5. No regression to the existing five sidebar items, and no changes to any non-org sidebar.

## Out of Scope

- Changes to the Providers page itself.
- Changes to routing — the `org_providers` route already exists.
- RBAC / authorization — handled by the existing route + page.
- Reordering existing sidebar items.
