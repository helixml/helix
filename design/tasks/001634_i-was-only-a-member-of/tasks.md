# Implementation Tasks

- [ ] Add `ResourceProject` to `RoleRead.Rules[0].Resources` in `api/pkg/types/authz_roles.go`
- [ ] Add `ResourceProject` to `RoleWrite.Rules[0].Resources` in `api/pkg/types/authz_roles.go`
- [ ] Make `seedOrganizationRoles` upsert by `(organization_id, name)` instead of insert-only
- [ ] Call `seedOrganizationRoles` for every existing org during server startup so production role configs get refreshed
- [ ] Add `authz_test.go` tests: org member sees project via team grant for each of `admin`, `read`, `write`; does NOT see it with no grant
- [ ] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes
- [ ] `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1` passes
- [ ] End-to-end in inner Helix at `http://localhost:8080`: org-member-via-team-admin sees the project on the org page
