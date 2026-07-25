# Design: Restore Projects as First Nav Item and Default Landing Page

## Overview

Small, frontend-only change touching two files. One moves an object literal to the top
of an array; the other simplifies a two-branch function to a constant. No new
abstractions, no state, no API changes.

## Files to Change

| File | Change |
|---|---|
| `frontend/src/components/orgs/UserOrgSelector.tsx` | Move the Projects entry to the head of `baseButtons` (currently ~lines 414–420, target: before the alpha-gated Org Chart block at ~lines 393–399). |
| `frontend/src/utils/organizations.ts` | `orgLandingRoute()` returns `'org_projects'` unconditionally (lines 8–12). |

## Key Decisions

**1. Change the nav order in place rather than reverting `8bf905644`.**
That commit also renamed "Org" → "Org Chart" and removed Providers from the rail — both
still wanted. A targeted move of the Projects block is the smaller, safer edit.

**2. Keep `orgLandingRoute()` as a function instead of inlining `'org_projects'` at the
four call sites.** It stays a single choke point if the landing page ever becomes
configurable again, and the diff stays to one file. Its body becomes:

```ts
// Route to land on after creating or selecting an org. Projects is the default
// home page for everyone; the Org Chart is reachable from the nav rail.
export function orgLandingRoute(_user?: { alpha_features?: string[] } | null): string {
  return 'org_projects'
}
```

The parameter is retained so the four call sites
(`OrgsTable.tsx:188`, `EditOrgWindow.tsx:170`, `UserOrgSelector.tsx:284`, `:292`) need no
edits.

**3. Keep `isHelixOrgEnabled()` alive by using it in `UserOrgSelector`.**
Once `orgLandingRoute()` stops branching, `isHelixOrgEnabled()` has no callers.
`UserOrgSelector.tsx:385` currently duplicates the same check inline:

```ts
const helixOrgEnabled = account.user?.alpha_features?.includes('helix-org') ?? false
```

Replace it with `isHelixOrgEnabled(account.user)`. This removes a duplicated literal and
keeps the exported helper meaningful. (Alternative — delete the helper — is listed as an
open question.)

**4. `router.tsx:661` needs no change.** The `/` bootstrap already hardcodes
`router.navigate('org_projects', { org_id: storedOrg })`. It was previously an
*inconsistency* (alpha users got Projects from `/` but Org Chart from a switch); after
this change it is simply consistent with `orgLandingRoute()`. Leave it as-is.

## Target Nav Rail Order

```
1. Projects        (Kanban)     -> org_projects            [moved to top]
2. Org Chart       (Network)    -> helix_org_chart         [alpha: helix-org]
3. Agents          (Bot)        -> agents
4. Chat            (MessageCircle) -> chat
5. Tasks           (Clock)      -> tasks
6. Sandbox         (Container)  -> sandboxes
7. Settings        (Settings)   -> org_general             [only if currentOrgSlug]
```

The Org Chart comment block needs its wording updated — it currently says "Leads the
rail as the primary org-level surface", which will no longer be true. Change to
something like "Slots in under Projects with the other primary org-level surfaces."

## Other Landing-Page Entry Points (verified, no change needed)

These already hardcode `org_projects` and therefore already do the right thing:

- `frontend/src/router.tsx:661` — `/` bootstrap with a stored org
- `frontend/src/components/system/SidebarContextHeader.tsx:18–22` — org name click
- `frontend/src/pages/NotFound.tsx:64–69` — "Back to Projects"
- `frontend/src/components/system/AccessDenied.tsx:37` — "Back to Projects"
- `frontend/src/pages/Onboarding.tsx:1055` — onboarding dismiss
- `frontend/src/pages/OrgSettings.tsx:177`

Login (`Login.tsx:56–74`) falls back to `'/'`, which routes through the bootstrap above.

## Codebase Notes for Future Agents

- The **left icon rail is not a router config** — it is a `useMemo`-built array called
  `navigationButtons` inside `UserOrgSelector.tsx` (~line 387). Order in that array =
  visual order top-to-bottom. Settings is `push`ed on afterwards conditionally.
- The **secondary/context sidebar** is separate: `Layout.tsx:443–487`
  (`getSidebarForRoute()`) picks between `ProjectsSidebar`, `OrgSidebar`, etc. Nothing
  here changes.
- There is **no `path: '/'` route**. `org_projects` is `/orgs/:org_id` and doubles as the
  org root. `/` is resolved imperatively in `router.tsx:634–666` after `router.start()`.
- Alpha features are read from `account.user.alpha_features` (string array); `helix-org`
  gates the Org Chart surface.
- Routing uses **router5** (`createRouter`, `router.navigate(name, params)`), so
  navigation is by route *name*, not path string.

## Testing

No unit tests currently cover `navigationButtons` or `orgLandingRoute` (nearest tests are
`src/contexts/account.test.tsx`, `src/pages/Login.test.tsx`). Verification is manual:

1. `cd frontend && yarn tsc --noEmit` (or the repo's existing lint/typecheck task) passes.
2. Load the app — Projects is the top rail icon.
3. Log in as a `helix-org` alpha user, switch orgs via the dropdown → lands on Projects,
   and the Org Chart icon is still present and still works.
4. Clear `localStorage.selected_org`, visit `/` → redirects to `/orgs`; pick an org →
   lands on Projects.

Adding a small unit test for `orgLandingRoute()` is optional and cheap; noted as a task.
