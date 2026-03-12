# Requirements: Customise Login SSO Copy for SaaS vs Enterprise

## Problem

Two issues with SaaS-vs-enterprise awareness in the frontend:

1. **Login copy**: The login page at app.helix.ml shows "Use your organization's single sign-on to access Helix." This is correct for enterprise customers who configured their own SSO, but misleading on the public SaaS where Helix manages the auth. A second instance in Session.tsx says "You can login with your Google account or your organization's SSO provider."

2. **Dead upsell/SaaS code to clean up**:
   - `useLiveInteraction.ts` has a hardcoded `window.location.hostname === "app.helix.ml"` check that gates a "stale interaction" timer. The `isStale` value it produces is returned from the hook but **never consumed by any UI component** — it's dead code. This was presumably wired to an upsell prompt that has since been removed.
   - `InteractionInference.tsx` has an `upgrade` prop that renders an "Upgrade" button (with a `queue_upgrade_clicked` analytics event). Both call sites in `Interaction.tsx` always pass `upgrade={false}`, so this button can never appear — also dead code.

## User Stories

1. **As a Helix Cloud user**, I want the login page to say something accurate like "Sign in to Helix" rather than implying I have my own SSO, so I'm not confused about whose auth system I'm using.

2. **As an enterprise customer**, I want the login page to reference "your organization's SSO" because my company has configured its own identity provider, and this messaging is correct and helpful.

## Locations to Change

| File | Line | Current Text / Code | Action |
|------|------|-------------|--------|
| `frontend/src/pages/Login.tsx` | ~426 | `"Use your organization's single sign-on to access Helix."` | Conditionalise for SaaS vs enterprise |
| `frontend/src/pages/Login.tsx` | ~444 | `"Sign in with SSO"` | Conditionalise for SaaS vs enterprise |
| `frontend/src/pages/Session.tsx` | ~1628 | `"You can login with your Google account or your organization's SSO provider."` | Conditionalise for SaaS vs enterprise |
| `frontend/src/hooks/useLiveInteraction.ts` | ~42-44, ~101-117 | `isAppTryHelixDomain` hostname check + stale timer + `isStale` state | Remove entirely (dead code) |
| `frontend/src/components/session/InteractionInference.tsx` | ~63, ~81, ~462-481 | `upgrade` prop + "Upgrade" button + `queue_upgrade_clicked` event | Remove entirely (dead code, always `false`) |
| `frontend/src/components/session/Interaction.tsx` | ~218, ~302 | `upgrade={false}` on both `<InteractionInference>` call sites | Remove the prop (no longer exists) |

## Acceptance Criteria

1. On Helix Cloud (app.helix.ml), the login page shows neutral copy that doesn't reference "your organization" — e.g. "Sign in to your Helix account" / "Sign in".
2. On enterprise/self-hosted deployments with OIDC, the existing "your organization's SSO" wording is preserved.
3. The Session.tsx login prompt also adapts its text based on the same condition.
4. The `isAppTryHelixDomain` hostname check, `isStale` state, stale timer, and `isStale` field on `LiveInteractionResult` are all removed from `useLiveInteraction.ts`.
5. The `upgrade` prop, "Upgrade" button, and `queue_upgrade_clicked` event are removed from `InteractionInference.tsx`, and the `upgrade={false}` props are removed from `Interaction.tsx`.
6. No new API endpoints or config fields are needed — use existing `config` data to distinguish deployments.

## How to Distinguish SaaS from Enterprise

The `/api/v1/config` endpoint returns a `ServerConfigForFrontend` object. Observed on app.helix.ml:
- `auth_provider`: `"oidc"`
- `billing_enabled`: `true`
- `stripe_enabled`: `true`
- `edition`: (empty/undefined)

Recommendation: Use `edition === "cloud"` — it's the most semantically correct and future-proof. Requires a one-line infra change to set `HELIX_EDITION=cloud` in the SaaS deployment's environment. The `edition` field already exists end-to-end (Go config → API → frontend types).