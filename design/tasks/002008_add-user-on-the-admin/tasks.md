# Implementation Tasks

- [ ] Check `git log --all --grep="OIDC"` in the helix repo — if task 002007's PR has merged, close this task as a duplicate and skip the rest
- [ ] Add OIDC guard in `api/pkg/server/handlers.go` `createUser` (after the admin check, before request decoding): return `system.NewHTTPError400` with the explanation message when `apiServer.Cfg.Auth.Provider == types.AuthProviderOIDC`
- [ ] Add a Go unit test in `api/pkg/server/` (gomock + suite per CLAUDE.md) asserting: response is HTTP 400 with the expected message substring AND `store.CreateUser` is not called (`Times(0)`) when provider is OIDC
- [ ] Run `CGO_ENABLED=1 go test -v -run <NewTestName> ./pkg/server/ -count=1` and confirm green; run `go build ./pkg/server/...` to confirm Go still compiles
- [ ] In `frontend/src/components/dashboard/UsersTable.tsx`: import `useGetConfig` from `../../services/userService` and `TypesAuthProvider` from `../../api/api`; compute `isOIDC = config?.auth_provider === TypesAuthProvider.AuthProviderOIDC`; wrap the `<Box>` containing the Create User button (lines ~290-294) in `{!isOIDC && (...)}`
- [ ] Do NOT modify `CreateUserDialog.tsx` — it remains for regular-auth mode and is unreachable in OIDC mode because the entry button is gone
- [ ] Run `cd frontend && yarn build` and confirm it compiles cleanly
- [ ] Test in inner Helix (regular mode, default): register/login as admin, navigate to Admin Dashboard → Users, confirm Create User button still renders and the existing creation flow works end-to-end (no regression)
- [ ] If `docker-compose.oidc.yaml` can be used to bring up an OIDC-mode instance, log in as admin and confirm the Create User button does NOT render; save a screenshot to `screenshots/01-oidc-no-create-button.png`. Otherwise add an explicit "WARNING: OIDC UI path NOT verified in browser, only by code inspection + unit test" note in the PR description per CLAUDE.md
- [ ] Open PR with conventional commit message `fix(api): block local user creation when OIDC is configured` and `fix(frontend): hide Create User button in OIDC mode`; link this spec folder in the PR description
- [ ] Watch CI on the PR (`gh pr checks <num>` or Drone MCP); investigate and fix any failures yourself before pinging the user
