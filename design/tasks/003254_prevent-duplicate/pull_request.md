# fix(onboarding): keep one canonical org across checkout round trip

## Summary

A signup that left for Stripe checkout could come back to onboarding and create a second organization for the same user (observed 2026-09-15: the round-trip re-entry offered the create form again, the backend name-uniqueness loop accepted the re-create as `name-1`, and a manual credit grant then targeted the wrong org).

Root cause is the onboarding org-create action (frontend/src/pages/Onboarding.tsx `handleCreateOrg`), the one web-signup path that creates organizations: it POSTed unconditionally, and between org creation and checkout the canonical org id lived only in React state plus the localStorage draft. Any state loss on return (redirect, refresh, lost draft) re-opened the create path.

Two structural changes at that ownership point:

1. `handleCreateOrg` now resolves instead of re-creating. If a canonical organization already exists — an already-selected `createdOrg`, the `org_id` from the round-trip continuation state (return URL), or any organization the viewer owns — it selects that org and advances the step. It only POSTs for signups that own no organization, so existing non-checkout paths (self-hosted, billing disabled, invited members selecting an existing org) are unchanged.
2. After a real create, the canonical org id is pinned into the URL (`/onboarding?org_id=<id>&created_org=true`) via `history.replaceState`, so the id survives redirect and refresh even without the draft.

No cleanup path for duplicate rows was added — the round trip can no longer create a second organization. No backend change: no other client reaches `POST /api/v1/organizations` during the round trip, and the name-suffix loop remains correct for the post-onboarding orgs page.

## Testing

- Two new falsifiable regression tests in `frontend/src/pages/Onboarding.test.tsx`, both verified to fail against the pre-fix code and pass after:
  - create resolves to the owned organization instead of a duplicate when round-trip state was lost (also asserts the trial checkout then targets the canonical org id);
  - the created org id is pinned into the URL and restores the canonical org on a draft-less refresh, with no second create call.
- Full `Onboarding.test.tsx` suite: 27/27 pass (25 pre-existing + 2 new). Related suites (Login, account context, useOrganizations): 61/61 pass. `yarn build` green. Full vitest runs show unrelated pre-existing flakes under load (jsdom XHR AggregateError, a different file each run) — none in touched files.
- End-to-end in the inner Helix dev stack at localhost:8080 with a fresh user: registered, created an org (URL pinned to `?org_id=org_…&created_org=true`, DB shows exactly 1 org), reloaded the page (org restored from the URL, no create form), then reproduced the broken-restore state (create-mode draft with lost org id, params stripped) and clicked "Create organization" — it resolved to the canonical org and the DB still showed exactly 1 organization.
- Not tested live: a real Stripe payment round trip (no Stripe keys in the dev stack); the checkout return path is covered by the unit tests on org targeting and restore.
