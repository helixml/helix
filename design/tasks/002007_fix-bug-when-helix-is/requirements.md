# Requirements: Fix admin "Create User" when OIDC is configured

## Bug

When Helix is configured with `AUTH_PROVIDER=oidc`, an admin clicks **Create User** on the Admin Dashboard, fills in email/password, and submits. Helix writes a local DB row with `auth_provider=regular` and a bcrypt password hash. The OIDC provider knows nothing about this user, so the user has **no way to log in**:

- The login screen only shows the OIDC button (the regular email/password form is hidden when `config.auth_provider === 'oidc'` — see `frontend/src/pages/Login.tsx:93`).
- The OIDC IdP has no account for this email, so the OIDC flow fails.
- The local password hash is unreachable.

The local row is also misleading — it makes it look like the user exists, but they cannot authenticate.

## User stories

**As an admin running Helix with OIDC**, when I click "Create User" I want clear guidance that user provisioning is managed by my OIDC provider, so I do not create dead local accounts that confuse my user list.

**As a new user added by my admin via the OIDC provider**, when I log in via OIDC for the first time, I want Helix to provision my local row automatically (this already works via JIT in `api/pkg/auth/oidc.go:341-363`).

## Acceptance criteria

1. **Backend rejects local user creation in OIDC mode.** `POST /api/v1/users` returns HTTP 400 with a clear message ("User creation is managed by your OIDC provider...") when `cfg.Auth.Provider == AuthProviderOIDC`. No local DB row is written.
2. **Frontend dialog explains the situation in OIDC mode.** When `config.auth_provider === 'oidc'`, the `CreateUserDialog` shows an informational panel instead of the email/password form, naming the OIDC provider URL (`config.oidc_url` or similar) where the admin should go. The Create button is hidden.
3. **Existing regular-auth flow is unchanged.** When `cfg.Auth.Provider == AuthProviderRegular`, behaviour is identical to today.
4. **JIT provisioning continues to work.** A user created in the OIDC provider lands in Helix's DB on first OIDC login (existing `OIDCClient.ValidateUserToken` behaviour).
5. **Backend test covers the OIDC rejection path** so the regression cannot reappear.

## Out of scope

- **Implementing a Keycloak/OIDC admin REST client** to actually create users in the IdP from Helix. Each IdP has a different admin API; that's a much larger feature. The right design today is to direct admins to their IdP. Reconsider only if a customer specifically requests it. (See `design.md` → "Could we reuse an existing OIDC CLI / client?" — verified there is no such CLI/client in the repo today; the existing `helix user` CLI calls the same buggy REST endpoint, and `api/pkg/auth/oidc.go` is consumer-side only with no admin API capability.)
- Bulk user import.
- Changing the regular-auth Create User UX.
