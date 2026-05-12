# Design: Hide "Create User" in OIDC mode

## Root cause (verified against current code)

`createUser` in `api/pkg/server/handlers.go:829-880` unconditionally builds a `types.User` with `AuthProvider: types.AuthProviderRegular` (line 870) and calls `apiServer.authenticator.CreateUser(ctx, newUser)` (line 874). The injected authenticator is `HelixAuthenticator`, which hashes the password and writes a Postgres row regardless of how the deployment is authenticating users. There is no check on `apiServer.Cfg.Auth.Provider` anywhere in this handler.

The frontend at `frontend/src/components/dashboard/UsersTable.tsx:290-294` always renders the button:

```tsx
<Box sx={{ mb: 2, display: "flex", justifyContent: "flex-end" }}>
  <Button variant="contained" color="secondary" startIcon={<AddIcon />} onClick={() => setCreateDialogOpen(true)}>
    Create User
  </Button>
</Box>
```

`auth_provider` is already exposed to the frontend via the existing `useGetConfig()` hook (used at `Login.tsx:80,93`) and is typed as `TypesAuthProvider` in the generated API client. No new endpoint or codegen run is needed.

JIT provisioning on first OIDC login already works (`api/pkg/auth/oidc.go:341-363`) — when an admin creates a user in the IdP, that user logs into Helix and a local row is materialised automatically. So the OIDC end-to-end flow is **already complete** without an admin UI; we just need to stop showing a UI that produces broken users.

## Decision: block, don't bridge

We **block** local user creation when OIDC is configured and direct admins to the OIDC provider's console. We do **not** add an IdP admin client. Reasons:

- **User explicitly said so**: *"Various providers (Okta, Google, Github) have different API's to create users, and we don't want to implement it."*
- **Each IdP has a different admin API** (Keycloak Admin REST, Okta Users API, Google Workspace Directory, Azure Graph, GitHub Enterprise SCIM). Building one is a feature; building five is ongoing maintenance.
- **JIT already covers create-on-first-login.** End-to-end the flow works once the user exists in the IdP.
- **A clear error beats a silent ghost user.** Today the bug creates a misleading row; a 400 with a clear message removes the foot-gun.

## Backend changes

**`api/pkg/server/handlers.go` — `createUser`**: insert the OIDC guard immediately after the admin check (between current lines 835 and 837):

```go
if apiServer.Cfg.Auth.Provider == types.AuthProviderOIDC {
    return nil, system.NewHTTPError400(
        "User creation is managed by your OIDC provider. " +
            "Create the user in the OIDC provider's admin console; " +
            "they will be provisioned in Helix on first login.")
}
```

That is the entire backend change. `apiServer.Cfg.Auth.Provider` and `types.AuthProviderOIDC` are already in scope (used in `auth.go` and `server.go` — see `grep "Cfg.Auth.Provider"` in `api/pkg/server/`). 400 (not 409) is correct because the request is invalid for the current server configuration; the caller should adjust.

The `helix user` CLI (in `api/pkg/cli/user/`) is a thin REST wrapper, so it inherits this guard automatically. There is no separate code path for CLI user creation.

## Frontend changes

**`frontend/src/components/dashboard/UsersTable.tsx`**:

1. Import `useGetConfig` from `../../services/userService` and `TypesAuthProvider` from `../../api/api` (mirror `Login.tsx:17,80,93`).
2. Inside the component: `const { data: config } = useGetConfig(); const isOIDC = config?.auth_provider === TypesAuthProvider.AuthProviderOIDC;`
3. Wrap the existing `<Box>` + `<Button>` block (lines 290-294) in `{!isOIDC && (...)}`.

Do **not** modify `CreateUserDialog.tsx`. The dialog stays as-is for regular-auth mode and is unreachable in OIDC mode because the only entry point is gone.

No generated API client regeneration is needed — `auth_provider` is already in the `ServerConfigForFrontend` payload and typed.

### Why hide the button rather than show an explanation

User request says "remove this option" — hiding the button outright is what was asked for and is cleaner than rendering a button that opens a dialog explaining you can't use it. Anyone confused about where to create users can read the Helix docs about OIDC provisioning.

## Files touched

| File | Change |
|------|--------|
| `api/pkg/server/handlers.go` | ~5 lines: OIDC guard at top of `createUser` |
| `api/pkg/server/handlers_test.go` (or new file) | Unit test for the OIDC guard |
| `frontend/src/components/dashboard/UsersTable.tsx` | Import `useGetConfig` + `TypesAuthProvider`; wrap button block in `{!isOIDC && ...}` |

No new dependencies. No type changes. No DB migration.

## Test plan

**Backend (Go unit test, gomock per CLAUDE.md)** — add to `api/pkg/server/`:
- Set `cfg.Auth.Provider = types.AuthProviderOIDC` (mirror the pattern at `auth_test.go:58`).
- Build a request as an admin and call `createUser`.
- Assert response is `*system.HTTPError` with status 400 and the expected message substring.
- Assert `store.CreateUser` was NOT invoked (gomock `EXPECT().CreateUser(...).Times(0)`).
- Add a sibling positive test that confirms `cfg.Auth.Provider = types.AuthProviderRegular` still calls through and creates a user (or rely on existing handler test coverage if it exists — check first).

Run with `CGO_ENABLED=1 go test -v -run TestCreateUserOIDC ./pkg/server/ -count=1` per the CGo notes in `helix/CLAUDE.md`.

**Frontend (manual, in inner Helix per CLAUDE.md)** — primary verification path:

1. Inner Helix at `http://localhost:8080` ships in **regular auth mode** by default. Register `test@helix.ml` / `helixtest`, complete onboarding, navigate to the Admin Dashboard → Users → confirm the **Create User** button is **visible** and the existing flow still works (no regression).
2. If feasible to flip the inner Helix to OIDC mode (`docker-compose.oidc.yaml` exists in repo root — investigate if it can be brought up alongside or instead of dev compose), repeat steps and confirm the **Create User** button is **not visible**. Capture a screenshot to `screenshots/`.
3. If switching the inner Helix to OIDC is not feasible, flag this explicitly per CLAUDE.md ("WARNING: OIDC UI path verified by code inspection + unit test only, NOT by browser test in OIDC mode") and rely on the unit test for OIDC coverage.

## Notes for the implementer

- **Check 002007 first.** This task and `002007_fix-bug-when-helix-is` are the same bug. Run `git log --all --grep="OIDC"` in `helix.git` before starting — if 002007's PR has merged, there is nothing to do here and the task should close as a duplicate.
- `apiServer.Cfg` is the existing `*config.ServerConfig` field on `HelixAPIServer` (capital C — confirmed by `grep "apiServer.Cfg"`).
- `types.AuthProviderOIDC` lives at `api/pkg/types/authz.go:131`.
- Don't add `gocloak`, `okta-sdk-golang`, `github.com/google/google-api-go-client`, or any IdP admin SDK. If you find yourself reaching for one, this design has been changed without updating the spec — stop and re-read.
- The `<Box>` at `UsersTable.tsx:290` only exists to right-align the button. Removing the whole Box (not just the Button) when `isOIDC` keeps the page header clean.
