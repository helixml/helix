# Implementation Tasks

- [ ] In `api/pkg/types/authz_roles.go`, add `ResourceProject` to `RoleRead.Rules[0].Resources`
- [ ] In `api/pkg/types/authz_roles.go`, add `ResourceProject` to `RoleWrite.Rules[0].Resources`
- [ ] Add a `syncOrganizationRoles(ctx, orgID)` function in `api/pkg/server/organization_handlers.go` that upserts canonical role configs (update existing by name, create if missing) for an org
- [ ] Call `syncOrganizationRoles` from `createOrganization` instead of `seedOrganizationRoles` (make seed idempotent)
- [ ] Add a startup migration in the server init path that calls `syncOrganizationRoles` for all existing orgs so stale role configs in production are repaired
- [ ] Add unit test: org member in a team with admin grant on a project can see that project via `listOrganizationProjects`
- [ ] Add unit test: org member in a team with read grant on a project can see that project via `listOrganizationProjects`
- [ ] Add unit test: org member in a team with no grant on a project cannot see that project via `listOrganizationProjects`
- [ ] Run `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and confirm no compile errors
- [ ] Verify fix end-to-end in the inner Helix at `http://localhost:8080`: create org, add user as member, create team, add user to team, grant team admin on project, confirm project is visible to the member
