# Implementation Tasks

- [x] In `api/pkg/types/authz_roles.go`, append `ResourceProject` to `RoleRead.Rules[0].Resources`
- [x] In `api/pkg/types/authz_roles.go`, append `ResourceProject` to `RoleWrite.Rules[0].Resources`
- [x] Verified `autoMigrateRoleConfig` (api/pkg/store/organization_roles.go:12) already syncs `roles.config` from `types.Roles` at every API server startup — no new sync code needed; my earlier spec was wrong about this
- [x] Added `AuthzProjectViaTeamSuite` in `api/pkg/server/authz_test.go` with 4 tests covering admin/read/write grants and the no-grant negative case
- [x] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes
- [x] `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1` passes (all 27 tests, including 4 new ones; no regressions in existing suites)
- [x] Commit and push to `fix/001634-team-project-visibility` branch on helix-4
