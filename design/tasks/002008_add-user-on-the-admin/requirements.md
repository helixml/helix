# Requirements: Hide "Create User" in OIDC mode

## Bug

The Admin Dashboard's **Create User** button (`frontend/src/components/dashboard/UsersTable.tsx:290-294`) is always visible. When Helix is configured with `AUTH_PROVIDER=oidc`, clicking it lets an admin write a local DB row with `auth_provider=regular` and a bcrypt password hash, but:

- The login screen hides the email/password form in OIDC mode (`frontend/src/pages/Login.tsx:93`), so the new user cannot log in locally.
- The OIDC IdP knows nothing about the email, so SSO login also fails.
- The local row is misleading — it makes the user appear "created" in the admin list while they have no working path to authenticate.

User provisioning must happen in the OIDC provider's admin console (Okta, Google, GitHub, Azure AD, Keycloak, etc.). Each IdP has a different admin API and we are explicitly **not** going to implement them. The fix is to remove the option from the UI when OIDC is active.

## User stories

**As an admin running Helix with OIDC**, I do not want to see a "Create User" button on the Admin Dashboard, so that I am not tempted to create dead local accounts and instead manage users in my OIDC provider.

**As a new user added by my admin via the OIDC provider**, when I sign in via OIDC for the first time, I expect Helix to provision my local row automatically (this already works via JIT in `api/pkg/auth/oidc.go:341-363` — no change required).

## Acceptance criteria

1. **Button hidden in OIDC mode.** When the server-reported `auth_provider === 'oidc'`, the "Create User" button at `UsersTable.tsx:290-294` does not render. The wrapping `<Box>` (which exists only to right-align the button) is also omitted so the page header doesn't have an empty spacer row.
2. **Backend rejects local user creation in OIDC mode (defence in depth).** `POST /api/v1/users` returns HTTP 400 with a clear message ("User creation is managed by your OIDC provider; create the user in the OIDC provider's admin console — they will be provisioned in Helix on first login.") when `cfg.Auth.Provider == types.AuthProviderOIDC`. No local DB row is written. This guards against direct API calls, the `helix user` CLI, and stale frontend bundles.
3. **Regular auth flow unchanged.** When `cfg.Auth.Provider == types.AuthProviderRegular`, the button renders and clicking it creates a local user exactly as today.
4. **JIT provisioning continues to work.** A user created in the OIDC provider lands in Helix's DB on first OIDC login (existing behaviour at `api/pkg/auth/oidc.go:341-363`, no change).
5. **Backend test covers the OIDC rejection path** so this regression cannot reappear silently.

## Out of scope

- **Implementing IdP-specific admin REST clients** (Keycloak Admin API, Okta `/api/v1/users`, Google Workspace Directory API, GitHub Enterprise SCIM, Azure Graph). User explicitly excluded this: *"we don't want to implement it"*. Each IdP has a distinct API and a separate auth model — this is a multi-week feature, not a bug fix.
- **Bulk user import** (separate feature).
- **Changing the regular-auth Create User UX** — leave the existing dialog and styling alone.
- **Page header copy tightening** — the page title/description above the table is fine as-is.

## Note on duplication with task 002007

Task `002007_fix-bug-when-helix-is` covers the same bug and has design docs but no merged implementation in `helix.git` yet (verified via `git log` — only `helix-specs` commits exist). If 002007's PR lands first, this task may be a no-op; check `git log --grep="OIDC"` in `helix.git` before starting implementation. The plan here is intentionally identical so either task closes the bug.
