# Implementation Tasks

- [ ] In `api/pkg/types/authz_roles.go`, append `ResourceProject` to `RoleRead.Rules[0].Resources`
- [ ] In `api/pkg/types/authz_roles.go`, append `ResourceProject` to `RoleWrite.Rules[0].Resources`
- [ ] Change `seedOrganizationRoles` in `api/pkg/server/organization_handlers.go` to upsert by `(organization_id, name)` instead of insert-only (update `Config` and `Description` when a matching name already exists)
- [ ] In server startup, walk `Store.ListOrganizations` and call `seedOrganizationRoles` for each so existing orgs (including `the-linux-foundation`) get the corrected canonical role configs
- [ ] Add tests in `api/pkg/server/authz_test.go`: org member in a team with `admin` / `read` / `write` grant on a project sees the project; org member with no team grant does not
- [ ] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes
- [ ] `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1` passes
- [ ] E2E in inner Helix at `http://localhost:8080`: org-member-via-team-admin sees the project on the org page
