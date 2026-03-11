# Requirements: Customise Login SSO Copy for SaaS vs Enterprise

## Problem

The login page at app.helix.ml (Helix Cloud / SaaS) shows "Use your organization's single sign-on to access Helix." and "Sign in with SSO". This wording is appropriate for enterprise customers who have configured their own SSO provider, but misleading on the public SaaS where the SSO provider (Keycloak) is managed by Helix — it's not "your organization's" SSO, it's Helix's own auth.

A second instance exists in Session.tsx where a login prompt says "You can login with your Google account or your organization's SSO provider."

## User Stories

1. **As a Helix Cloud user**, I want the login page to say something accurate like "Sign in to Helix" rather than implying I have my own SSO, so I'm not confused about whose auth system I'm using.

2. **As an enterprise customer**, I want the login page to reference "your organization's SSO" because my company has configured its own identity provider, and this messaging is correct and helpful.

## Locations to Change

| File | Line | Current Text | Context |
|------|------|-------------|---------|
| `frontend/src/pages/Login.tsx` | ~426 | `"Use your organization's single sign-on to access Helix."` | OIDC login page description |
| `frontend/src/pages/Login.tsx` | ~444 | `"Sign in with SSO"` | OIDC login button label |
| `frontend/src/pages/Session.tsx` | ~1628 | `"You can login with your Google account or your organization's SSO provider."` | Login prompt in shared session view |

## Acceptance Criteria

1. On Helix Cloud (app.helix.ml), the login page shows neutral copy that doesn't reference "your organization" — e.g. "Sign in to your Helix account" / "Sign in".
2. On enterprise/self-hosted deployments with OIDC, the existing "your organization's SSO" wording is preserved.
3. The Session.tsx login prompt also adapts its text based on the same condition.
4. No new API endpoints or config fields are needed — use existing `config` data to distinguish deployments.

## How to Distinguish SaaS from Enterprise

The `/api/v1/config` endpoint returns a `ServerConfigForFrontend` object. Observed on app.helix.ml:
- `auth_provider`: `"oidc"`
- `billing_enabled`: `true`
- `stripe_enabled`: `true`
- `edition`: (empty/undefined)

Options for the check (in order of preference):
1. **`billing_enabled === true`** — SaaS has billing; enterprise self-hosted typically does not. Already available in config, no backend change needed.
2. **`edition === "cloud"`** — More explicit, but currently not set on app.helix.ml. Would require a backend config change to set `HELIX_EDITION=cloud` in the SaaS deployment.

Recommendation: Use `edition === "cloud"` — it's the most semantically correct and future-proof. Requires a one-line infra change to set `HELIX_EDITION=cloud` in the SaaS deployment's environment.