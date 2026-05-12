# Implementation Tasks

- [ ] In `api/pkg/types/authz_roles.go`, add `ResourceProject` to `RoleRead.Rules[0].Resources`
- [ ] In `api/pkg/types/authz_roles.go`, add `ResourceProject` to `RoleWrite.Rules[0].Resources`
- [ ] Add `syncCanonicalOrgRoles(ctx)` in `api/pkg/server/organization_handlers.go` that walks all orgs and upserts roles whose `name` matches a canonical role in `types.Roles` to the canonical `Config` (insert if missing)
- [ ] Wire `syncCanonicalOrgRoles` into the server startup path (next to other one-shot init in `server.go`); log how many orgs/roles were updated
- [ ] Add `TestProjectAccessViaTeamWithRead`, `TestProjectAccessViaTeamWithWrite`, `TestProjectAccessViaTeamWithAdmin`, `TestProjectAccessViaTeamNoGrant` in `api/pkg/server/authz_test.go` using the existing `gomock` + `MockStore` pattern; mocks must return a real team ID from `ListTeams` and a grant carrying the role under test from `ListAccessGrants`
- [ ] Run `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` — must succeed
- [ ] Run `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1` — new tests must pass
- [ ] End-to-end test in inner Helix at `http://localhost:8080`: create org, second user as member, team, add member to team, project, grant team admin → second user must see the project. Repeat with `read` and `write` grants
- [ ] If the admin-role case still fails after the code fix, query Postgres (`docker exec helix-postgres-1 psql -U postgres -d postgres -c "..."`) for the actual `roles.config` and `access_grants` rows of `the-linux-foundation` to confirm the data shape, then file/document a follow-up
