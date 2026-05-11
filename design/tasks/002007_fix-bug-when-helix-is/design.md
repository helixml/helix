# Design: Fix admin "Create User" when OIDC is configured

## Root cause

`createUser` in `api/pkg/server/handlers.go:818-869` unconditionally builds a `types.User` with `AuthProvider: types.AuthProviderRegular` and calls `apiServer.authenticator.CreateUser(ctx, newUser)`. The injected `authenticator` is always `HelixAuthenticator` (built once in `api/cmd/helix/serve.go:294`, regardless of auth provider), and its `CreateUser` (`api/pkg/auth/helix_authenticator.go:87`) hashes the password and writes a Postgres row. Nothing checks whether OIDC is the configured provider, and nothing reaches out to the OIDC IdP.

Helix already supports the inverse direction (IdP → Helix) via JIT provisioning in `api/pkg/auth/oidc.go:341-363`: on first OIDC login, the user info from the IdP is materialised as a local DB row keyed by the OIDC subject. So when OIDC is enabled, **the IdP is the source of truth for users** and Helix should not create local-auth accounts at all.

## Decision: block, don't bridge

**We block local user creation when OIDC is configured and direct the admin to the OIDC provider.** We do **not** add a Keycloak (or generic OIDC) admin REST client. Reasons:

- **Each IdP has a different admin API.** Keycloak, Okta, Azure AD, Google Workspace, Auth0 all differ — building one is a non-trivial integration; building five is a maintenance burden out of scope for a bug fix. CLAUDE.md is explicit: don't add hacks/workarounds, root-cause and fix properly. Properly here means recognising the IdP owns user provisioning.
- **JIT already covers the create-on-first-login flow.** Once the admin creates the user in the IdP, the user logs in and Helix provisions them automatically. End-to-end this works.
- **A clear error is better than a silent ghost user.** Today the bug creates a row that misleads the admin into thinking the user exists. A 400 with a clear message removes the foot-gun.

### Could we reuse an existing OIDC CLI / client?

Reviewer asked whether an existing OIDC CLI in the repo could be used to create users in the IdP. Verified — it cannot:

- **`helix oidc` does not exist.** The registered subcommands (`api/cmd/helix/root.go:43-68`) are: `app, chat, knowledge, fs, secret, project, sandbox, spectask, mcp, model, provider, organization, roles, system, team, member, user`. No `oidc`.
- **`helix user` CLI exists** (`api/pkg/cli/user/`) with `list`, `delete`, `reset-password` subcommands — but **no `create` subcommand**, and every command is a thin wrapper over the same Helix REST API (`/api/v1/users`, `/api/v1/admin/users/...`). Adding `helix user create` would call the same buggy `createUser` handler — same problem.
- **Helix's OIDC code is consumer-side only.** `api/pkg/auth/oidc.go` uses `coreos/go-oidc` to verify tokens and call the userinfo endpoint. There is no admin API client (no `gocloak`, no `okta-sdk-golang`, etc.), no admin client credentials configured, and no code that POSTs `/admin/realms/.../users` to anything.
- **Keycloak ships its own `kcadm.sh`**, but it is (a) Keycloak-specific, (b) not bundled with or invoked by Helix, and (c) shelling out to a CLI from inside an HTTP handler is the wrong pattern (auth, error handling, deployment).

So there is no existing capability to reuse. Bridging would still be a net-new integration with all the same trade-offs as Option B in the original analysis.

If a customer later wants Helix to be the user-creation UI for Keycloak specifically, that's a separable feature with its own spec — and would build on top of this fix (add configured Keycloak admin credentials, a `keycloak.AdminClient`, and a separate code path that creates in IdP first then mirrors locally). It is not in scope here.

## Backend changes

**`api/pkg/server/handlers.go` — `createUser`** (around line 822, after the admin check):

```go
if apiServer.Cfg.Auth.Provider == types.AuthProviderOIDC {
    return nil, system.NewHTTPError400(
        "User creation is managed by your OIDC provider. " +
        "Create the user in the OIDC provider's admin console; " +
        "they will be provisioned in Helix on first login.")
}
```

That's the entire backend change. No new types, no new dependencies. The check sits next to the existing admin check so it's obvious in the handler.

We keep using `system.NewHTTPError400` — 400 is appropriate because the request is invalid for the current server configuration; the client should adjust. (We considered 409 Conflict but the request isn't conflicting with state, it's incompatible with config.)

## Frontend changes

**`frontend/src/components/dashboard/CreateUserDialog.tsx`**:

1. Read auth provider via the existing `useGetConfig()` hook (already used in `Login.tsx:80`).
2. If `config.auth_provider === 'oidc'`, render an informational `<Alert severity="info">` inside `DialogContent` instead of the email/password fields, with text along the lines of:
   > User creation is managed by your OIDC provider. Create the user in your provider's admin console — they will appear here automatically on first login.
3. Hide the "Create" button in `DialogActions` when in OIDC mode (keep "Close").

**Caller (the dashboard page that opens this dialog)** — consider hiding the "Create User" button entirely in OIDC mode. Find the caller via `Grep` for `<CreateUserDialog`. If the button still makes sense as an entry point for the explanation, leave it; otherwise hide it. Decide during implementation.

No changes to the generated API client are needed (the endpoint signature is unchanged).

## Test plan

**Backend (Go unit test)** — add to `api/pkg/server/` a test that:
- Sets `cfg.Auth.Provider = types.AuthProviderOIDC`.
- Calls `createUser` as an admin.
- Asserts the response is HTTP 400 with the expected message.
- Asserts `store.CreateUser` was NOT called (gomock `.Times(0)`).

Mirror the pattern of existing handler tests in the same package (gomock + suite per CLAUDE.md).

**Frontend (manual, in inner Helix per CLAUDE.md)** — switch inner Helix to OIDC mode if available, log in as admin, click Create User, confirm the info panel renders and no API call is made when typing/clicking. Take screenshots for the spec folder.

If switching the inner Helix to OIDC is not feasible during implementation, flag explicitly per CLAUDE.md ("WARNING: NOT tested in OIDC mode in inner Helix") and rely on the unit test plus a regular-mode regression check.

## Files touched

| File | Change |
|------|--------|
| `api/pkg/server/handlers.go` | Add OIDC guard in `createUser` (~5 lines) |
| `api/pkg/server/handlers_test.go` (or new test file) | Add unit test for the OIDC guard |
| `frontend/src/components/dashboard/CreateUserDialog.tsx` | Branch on `config.auth_provider`, render info panel + hide submit in OIDC mode |
| Caller of `CreateUserDialog` (TBD during implementation) | Optionally hide "Create User" button in OIDC mode |

## Notes for the implementer

- `apiServer.Cfg` is the existing `*config.ServerConfig` field on `HelixAPIServer`; check the field name (it may be `cfg` lowercase) before using.
- `types.AuthProviderOIDC` is already defined at `api/pkg/types/authz.go:131`.
- `useGetConfig` returns a config object that already includes `auth_provider` (typed as `TypesAuthProvider` in the generated API client — see `frontend/src/api/api.ts:4596`). No regeneration needed.
- Don't import `gocloak` or any Keycloak admin SDK. If you find yourself reaching for one, this design has been changed without updating the spec — stop and re-read.
