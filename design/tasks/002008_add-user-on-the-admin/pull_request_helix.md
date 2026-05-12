# fix: hide "Create User" admin action when OIDC is configured

## Summary

When Helix runs in OIDC mode (`AUTH_PROVIDER=oidc`), the Admin Dashboard's **Create User** button still wrote a local DB row with `auth_provider=regular` and a bcrypt password hash. The new "user" had no working login path: the regular login form is hidden in OIDC mode, and the OIDC IdP knows nothing about them. The result was a misleading row that made admins think the user existed.

User provisioning in OIDC deployments has to happen in the OIDC provider's admin console (Okta, Google, GitHub, Azure AD, Keycloak, …). Each IdP has a different admin API and we are explicitly not implementing them here. JIT provisioning on first OIDC login (`api/pkg/auth/oidc.go`) already materialises the local row.

This PR removes the dead-end UI and rejects the API call when OIDC is the configured provider.

## Changes

- **`api/pkg/server/handlers.go`** — `createUser` returns HTTP 400 with a clear "user creation is managed by your OIDC provider" message when `Cfg.Auth.Provider == AuthProviderOIDC`. Guard sits next to the existing admin-only guard. No DB writes, no hash generation.
- **`api/pkg/server/create_user_handler_test.go`** (new) — gomock test asserts the OIDC guard returns 400 and that `Store.GetUser` and `authenticator.CreateUser` are never invoked. A second test confirms the existing non-admin 403 still fires first as a sanity check.
- **`frontend/src/components/dashboard/UsersTable.tsx`** — read `auth_provider` via the existing `useGetConfig()` hook (same pattern as `Login.tsx`), compute `isOIDC`, and wrap the **Create User** button + its right-aligning `<Box>` in `{!isOIDC && (...)}`.

No new dependencies. No DB migration. `CreateUserDialog.tsx` is unchanged — it stays as-is for regular auth and is unreachable in OIDC mode because the only entry point is gone.

## Test plan

- [x] `go build ./pkg/server/...` (clean)
- [x] `go test -v -run "TestCreateUser_OIDCMode_Rejected|TestCreateUser_NonAdmin_Forbidden" ./pkg/server/ -count=1` — both pass
- [x] `cd frontend && yarn build` — clean
- [ ] **WARNING: NOT verified in inner Helix browser.** The inner Helix stack was not running in the spec-task sandbox (helix-ubuntu image was still building per `/tmp/helix-startup.log`). The frontend change is a one-line conditional render that mirrors the exact `auth_provider === 'oidc'` pattern already used in `frontend/src/pages/Login.tsx:93`, and the backend path is fully covered by the new unit test. Reviewer or CI should confirm in a real OIDC deployment.

## Related

Same bug as task `002007_fix-bug-when-helix-is` (design docs only, no merged code). Either task fully closes the issue.
