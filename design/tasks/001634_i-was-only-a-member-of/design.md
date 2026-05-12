# Design

## Root Cause

`RoleRead` and `RoleWrite` in `api/pkg/types/authz_roles.go` enumerate `ResourceApplication`, `ResourceKnowledge`, `ResourceAccessGrants` — but **omit `ResourceProject`**. The matcher in `api/pkg/server/authz.go:evaluate` only allows when the rule's `Resources` contains the requested resource OR `ResourceAny`, so teams with read/write on a project fail `authorizeUserToProject`.

`RoleAdmin` uses `ResourceAny` and *should* work, but the user reports it doesn't for `the-linux-foundation`. Most likely: that org's `roles` row was seeded by an older codebase and its stored JSONB `config` is stale. Roles are seeded once at org creation (`seedOrganizationRoles`) and never re-synced when `types.Roles` changes.

## Fix

1. Add `ResourceProject` to `RoleRead.Rules[0].Resources` and `RoleWrite.Rules[0].Resources` in `api/pkg/types/authz_roles.go`.

2. Make role seeding idempotent and re-run on every server start: change `seedOrganizationRoles` to upsert (update existing row when `name` matches a canonical role; insert when missing), then call it for all orgs during `HelixAPIServer` init. Custom roles are untouched.

3. Add tests in `api/pkg/server/authz_test.go` (existing gomock pattern) for project access via team membership at each role level: `read`, `write`, `admin`, none.

## Verification

- `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
- `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1`
- End-to-end in inner Helix at `http://localhost:8080`: as org owner, create second user as member, create team, add member to team, create project, grant team admin on project. Log in as the member and confirm the project appears. Repeat with `read` and `write`.
