# Design

## Where the Error Comes From

`frontend/src/router.tsx` line 181:

```tsx
{ name: 'org_providers', path: '/orgs/:org_id/providers', … }
```

When any caller does `router.navigate('org_providers', { org_id: undefined })`, router5's `buildPath` throws:

```
Cannot build path: '/orgs/:org_id/providers' requires missing parameters { org_id }
```

That uncaught error bubbles to the React error boundary (and to `index.tsx`'s plain-HTML overlay on iPad), producing the crash the user reported.

## Existing Fallback Chain (account.tsx, lines 359–381)

```ts
const currentUrlOrgId = window.location.pathname.match(/^\/orgs\/([^/]+)/)?.[1]
const useOrgID = params.org_id
  || organizationToolsRef.current.organization?.name
  || currentUrlOrgId
```

This was added in commit `2a3b9fdd5` (Apr 9 2026). It is present in `2.11.0`. **It does not cover the case where the user is on a non-`/orgs/...` URL and `organization` hasn't loaded.** In that case `useOrgID` is still `undefined` and router5 throws.

## Fix

Two complementary changes.

### 1. Bullet-proof `orgNavigate` (primary fix)

Extend the fallback chain in `frontend/src/contexts/account.tsx` `orgNavigate`:

```ts
const currentUrlOrgId = window.location.pathname.match(/^\/orgs\/([^/]+)/)?.[1]
const firstOrgSlug = organizationToolsRef.current.organizations?.[0]?.name
const useOrgID = params.org_id
  || organizationToolsRef.current.organization?.name
  || currentUrlOrgId
  || firstOrgSlug

if (!useOrgID) {
  // No org context anywhere — send the user to the org picker rather than
  // throwing a router5 "missing parameter" error.
  console.warn('orgNavigate: no org_id resolvable, redirecting to /orgs', { routeName, params })
  router.navigate('orgs', {})
  return
}
```

This keeps the fix minimal:

- Adds **one** more fallback (the user's first org) — useful when `organizations` is loaded but `organization` (current org) is not.
- Adds a **final guard** so if no org is reachable at all (logged-in user with zero orgs, or a totally undecided state), we go to `/orgs` instead of crashing the page.
- Stays in one file — does not touch every individual call site.

### 2. Trim the trigger (secondary, defensive)

In `frontend/src/components/system/TokenUsageDisplay.tsx`, the "Add my own API Keys" button only makes sense when the user has an org context. Hide the button if there is no org to navigate to:

```ts
const account = useAccount()
const hasOrg = !!account.organizationTools.organization
  || !!account.organizationTools.organizations?.[0]
…
{account.serverConfig.providers_management_enabled && hasOrg && (
  <Button onClick={handleAddProviders}>Add my own API Keys</Button>
)}
```

This is belt-and-braces: even with the `orgNavigate` fix, hiding the button when there is genuinely nowhere to go is the more honest UX.

## Why Not Other Approaches

- **Make `org_id` optional in the route definition** — would silently break the providers page (it reads `org_id` from params); we'd need to add server-side defaulting too. Far more invasive.
- **Wrap every `orgNavigate` call site with an `if (orgId)` guard** — there are 20+ call sites; centralising the guard in `orgNavigate` itself is cleaner.
- **Defer rendering `TokenUsageDisplay` until org loads** — the panel itself works without an org (it shows quota usage); only the navigation button needs the org.

## Patterns / Notes for Future Agents

- All routes in `frontend/src/router.tsx` are org-scoped (`/orgs/:org_id/...`) — any nav helper that builds these must resolve `org_id` from somewhere.
- `account.orgNavigate` is the single chokepoint for org-scoped nav; harden there, don't sprinkle guards.
- `organizationTools.organization` (singular) is the **current** org; `organizationTools.organizations` (plural) is the **list**. Both can be empty during init; always have a fallback.
- The router5 error format is `Cannot build path: '<path>' requires missing parameters { <param> }`. Any future report of this format usually means a `router.navigate` call (or an `<a href>` built from `router.buildPath`) is missing a route param — search the codebase for that route name.
- `index.tsx` and `ErrorBoundary.tsx` show the full-page crash overlay. The overlay has a "Dismiss" button as of `2a3b9fdd5`, so even if we miss an edge case, the user can recover without reload.

## Verification

1. `cd frontend && yarn build` — compile cleanly.
2. In the inner Helix at `http://localhost:8080`:
   - Register / log in.
   - **Manually** navigate the URL bar to `/files` (a non-org page).
   - Open the user-org floating menu and confirm `TokenUsageDisplay` is mounted (it requires `quotas_enabled` server config — may need to set in dev).
   - In the React DevTools or via `account.orgNavigate('providers')` from the console, simulate the click.
   - Expected: navigates to `/orgs/<your-org>/providers`, no error overlay.
3. Edge case: a user with **no** orgs at all calls `orgNavigate('providers')` from `/onboarding`. Expected: console warn, navigate to `/orgs` (the picker), no crash.
4. Regression: from `/orgs/<slug>/projects`, click the Providers item in the org sidebar. Expected: still works.
