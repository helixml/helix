# Requirements

## Bug Report

A user on Helix SaaS (running release `2.11.0`) clicked something in the UI and the page crashed with the router5 error overlay:

```
Cannot build path: '/orgs/:org_id/providers' requires missing parameters { org_id }
```

This is router5 refusing to construct a URL because the `:org_id` parameter is missing from `router.navigate('org_providers', …)`. The user couldn't recover without reloading.

## Root Cause Hypothesis

`account.orgNavigate('providers')` resolves the org slug from a fallback chain (`params.org_id` → `organization?.name` → URL match `/^\/orgs\/([^/]+)/`). If the call is made from a page that is **not** under `/orgs/<slug>/…` and the current `organization` context hasn't loaded yet (or the user is between orgs), all three fallbacks return `undefined` and router5 throws.

The most likely trigger is the **"Add my own API Keys"** button inside `TokenUsageDisplay` (rendered in the user-org selector floating menu on every page where `account.user` is set). When the user is on `/files`, `/secrets`, `/api-reference`, `/orgs`, etc., that button calls `account.orgNavigate('providers')` with no `org_id` — and the URL fallback can't find one in the current path.

The earlier fix `2a3b9fdd5` ("iPad demo crashes — missing org_id nav error") added the URL-match fallback but does not cover non-org URLs, so the error still reproduces in 2.11.0+.

## User Stories

**As a Helix SaaS user with their token quota maxed out, on any non-org page (e.g. `/files`),** I want to click "Add my own API Keys" in the token usage panel and land on the providers page, **so that** I can add my own keys instead of seeing a crash overlay.

**As a Helix user whose org context is still loading,** I want clicking any nav button that targets an org-scoped route to wait or fall back gracefully, **so that** I never see a "missing mandatory parameter" router error.

## Acceptance Criteria

1. Clicking "Add my own API Keys" in `TokenUsageDisplay` from a non-org route (e.g. `/files`, `/secrets`, `/api-reference`, `/orgs`) navigates to `/orgs/<some-org>/providers` and never throws a router error.
2. `account.orgNavigate(routeName, …)` never calls `router.navigate` with `org_id: undefined`. If no org slug can be resolved from any source, it must do something safe — either route the user to `/orgs` (org picker) or no-op with a console warn — instead of letting router5 throw.
3. The error overlay no longer appears for this specific code path. Verified by manually navigating to `/files` (or any non-org page) and triggering the "Add my own API Keys" path with a quota-exceeded user.
4. No regression on the existing path: clicking "Providers" from the org sidebar on `/orgs/<slug>/projects` still works.
5. Fix is in a release ≥ `2.11.3` (or whatever the next patch line is) and customers on `2.11.0–2.11.2` can be advised to upgrade.

## Out of Scope

- Reworking router5 / migrating off router5.
- Redesigning the token-usage panel.
- Server-side providers handling.
