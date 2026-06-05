# Implementation Tasks

- [ ] Add OIDC guard at the top of `createUser` in `api/pkg/server/handlers.go` returning HTTP 400 with the explanation message when `cfg.Auth.Provider == types.AuthProviderOIDC`
- [ ] Add a Go unit test in `api/pkg/server/` that exercises the OIDC guard: asserts 400 response and verifies `store.CreateUser` is not called (gomock `.Times(0)`)
- [ ] Verify no other create-user code paths bypass the handler (grep for `authenticator.CreateUser` callers; confirm the only admin-facing path is `createUser`)
- [ ] In `frontend/src/components/dashboard/UsersTable.tsx`, read `config.auth_provider` via `useGetConfig()` and hide the "Create User" `<Button>` (and its wrapping `<Box>` at lines ~239-248) when value is `oidc`
- [ ] Do NOT modify `CreateUserDialog.tsx` — the dialog stays as-is for regular auth mode, and is unreachable in OIDC mode because the button is gone
- [ ] Run `cd frontend && yarn build` to confirm the frontend still compiles
- [ ] Run `go build ./pkg/server/...` from `api/` to confirm Go still compiles
- [ ] Test in inner Helix (regular mode): register/login as admin, confirm "Create User" button is still visible and the existing flow still works end-to-end
- [ ] If inner Helix can be reconfigured to OIDC mode, log in as admin, confirm the "Create User" button does NOT render, capture a screenshot into `screenshots/`; otherwise flag explicitly in the PR description that the OIDC UI path was only validated by code inspection + unit test
- [ ] Open PR with conventional commit message `fix(api): block local user creation when OIDC is configured` and link this spec
