# fix: orgNavigate crashes with missing org_id when called from non-org URLs

## Summary

A user on Helix SaaS (running release `2.11.0`) hit the router5 error overlay:

```
Cannot build path: '/orgs/:org_id/providers' requires missing parameters { org_id }
```

`account.orgNavigate('providers')` resolves the org slug from a fallback chain (`params.org_id` → current `organization?.name` → URL match for `/orgs/:slug/...`). When the user clicks **"Add my own API Keys"** in `TokenUsageDisplay` from a non-org URL (`/files`, `/secrets`, `/api-reference`, `/orgs`) and the current-organization context hasn't loaded yet, all three fallbacks return `undefined` and router5 throws. The earlier fix `2a3b9fdd5` ("iPad demo crashes — missing org_id nav error") added the URL-match fallback but does not cover this case.

This PR fixes the root cause in two complementary ways.

## Changes

- **`frontend/src/contexts/account.tsx`** — extend the `orgNavigate` fallback chain to also try `organizations[0]?.name` (the user's first loaded org), and add a final guard that redirects to `/orgs` (the org picker) with a `console.warn` instead of letting `router.navigate` crash on `org_id: undefined`.
- **`frontend/src/components/system/TokenUsageDisplay.tsx`** — hide the "Add my own API Keys" button when the user has no org context at all. Belt-and-braces: even with the `orgNavigate` fix, hiding a button that would have nowhere safe to go is the more honest UX.

## Test plan

- [x] `cd frontend && yarn build` passes.
- [x] Inner Helix: navigate to `/files` (non-org URL) — no console errors, page loads.
- [x] Inner Helix: navigate via the org sidebar to `/orgs/testorg/providers` — works (regression check).
- [x] Inner Helix: navigate to `/orgs` (the org picker, also non-org) — no errors.
- [ ] In production, a user who hits this code path on a non-org URL no longer sees the crash overlay; they either land on the providers page or the org picker.

## Screenshots

![Org picker page loads cleanly from non-org URL](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001989_fix-bug-user-on-helix/screenshots/01-orgs-page-no-error.png)

![Providers page reachable via the regression path](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001989_fix-bug-user-on-helix/screenshots/02-providers-page-loads.png)

## Notes for reviewer

- The fix is intentionally centralised in `orgNavigate` rather than guarding each of the ~20 call sites individually.
- The existing `index.tsx` / `ErrorBoundary.tsx` "Dismiss" overlay (`2a3b9fdd5`) means even any future missed edge case is recoverable without reload.
- Customers on `2.11.0`–`2.11.2` should be advised to upgrade once this lands in `≥ 2.11.3`.
