# Implementation Tasks

- [ ] Add OIDC guard at the top of `createUser` in `api/pkg/server/handlers.go` returning HTTP 400 with the explanation message when `cfg.Auth.Provider == types.AuthProviderOIDC`
- [ ] Add a Go unit test in `api/pkg/server/` that exercises the OIDC guard: asserts 400 response and verifies `store.CreateUser` is not called (gomock `.Times(0)`)
- [ ] Verify no other create-user code paths bypass the handler (grep for `authenticator.CreateUser` callers; confirm the only admin-facing path is `createUser`)
- [ ] In `frontend/src/components/dashboard/CreateUserDialog.tsx`, read `config.auth_provider` via `useGetConfig()` and render an `<Alert severity="info">` with guidance instead of the email/password form when value is `oidc`
- [ ] Hide the submit "Create" button in `DialogActions` when in OIDC mode (keep the close button working)
- [ ] Locate the caller that opens `CreateUserDialog` (grep `<CreateUserDialog`) and decide whether to hide the entry-point button in OIDC mode; document the decision in the PR description
- [ ] Run `cd frontend && yarn build` to confirm the frontend still compiles
- [ ] Run `go build ./pkg/server/...` from `api/` to confirm Go still compiles
- [ ] Test in inner Helix: register/login as admin in regular mode, confirm Create User flow still works end-to-end
- [ ] If inner Helix can be reconfigured to OIDC mode, test the OIDC path end-to-end and capture screenshots into `screenshots/`; otherwise flag explicitly in the PR description that the OIDC UI path was only validated by code inspection + unit test
- [ ] Open PR with conventional commit message `fix(api): block local user creation when OIDC is configured` and link this spec
