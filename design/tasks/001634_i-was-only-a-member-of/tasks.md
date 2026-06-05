# Implementation Tasks

## Step 1 — Add `ResourceProject` to read/write canonical roles

- [ ] In `api/pkg/types/authz_roles.go`, append `ResourceProject` to `RoleRead.Rules[0].Resources`
- [ ] In `api/pkg/types/authz_roles.go`, append `ResourceProject` to `RoleWrite.Rules[0].Resources`
- [ ] Verify `RoleAdmin` is unchanged (already covers via `ResourceAny`)

## Step 2 — Make role seeding idempotent + run for every existing org at startup

- [ ] If `Store.UpdateRole` does not exist, add it next to `CreateRole` in `api/pkg/store/` (mirrors signature of `CreateRole`; updates `Config` + `Description` fields)
- [ ] Rewrite `seedOrganizationRoles` in `api/pkg/server/organization_handlers.go` to upsert: list existing roles for the org by `(organization_id, name)`; for each canonical role in `types.Roles`, UPDATE the `Config`/`Description` if drift detected, INSERT if missing; leave custom roles (not in `types.Roles`) untouched. Use a JSON-equality helper (marshal both Configs and compare bytes) to detect drift
- [ ] Add a startup pass in `api/pkg/server/server.go` (next to other one-shot init): walk `Store.ListOrganizations(...)` and call `seedOrganizationRoles` for each org. Log a per-startup count `canonical org roles synced at startup orgs=N`. Log per-org failures at error level but do NOT fail startup for one bad org

## Step 3 — Tests

- [ ] Add `TestProjectViaTeam_AdminAllowed`, `TestProjectViaTeam_ReadAllowed`, `TestProjectViaTeam_WriteAllowed`, `TestProjectViaTeam_NoGrantDenied` in `api/pkg/server/authz_test.go`. Mirror the existing `AuthzAppSuite` pattern (`gomock`, `MockStore`, `expectOrgMember`). Mocks must return a non-empty team ID from `ListTeams` (regression-guard against empty-team-ID class of bug). Use `types.RoleAdmin`/`RoleRead`/`RoleWrite` Configs directly so the tests fail loudly if a future change removes `ResourceProject` from the canonical
- [ ] Add `TestSeedOrganizationRoles_UpdatesDriftedConfig` in `api/pkg/server/organization_handlers_test.go` (create file if absent). Setup: insert an org + roles; mutate the admin role's stored `Config` to a deliberately-broken value; call `seedOrganizationRoles` again; assert `Config` is restored to `types.RoleAdmin`. Also assert custom (non-canonical-named) roles are untouched

## Step 4 — Build + unit-test verification

- [ ] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes
- [ ] `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1` — all green including 4 new tests
- [ ] `CGO_ENABLED=1 go test -v -run TestSeedOrganizationRoles ./pkg/server/ -count=1` passes

## Step 5 — End-to-end verification in inner Helix

- [ ] Register `test@helix.local` / `testpass123` at `http://localhost:8080`, complete onboarding, create an org
- [ ] Register a second user (`member@helix.local`); add as **member** (not owner) of the org
- [ ] Create a team in the org; add the second user to the team
- [ ] Create three projects (P1, P2, P3) in the org; grant the team `admin` on P1, `write` on P2, `read` on P3
- [ ] Create a fourth project P4 with no grants
- [ ] Log in as the second user → org page (`/orgs/<slug>`) must show P1, P2, P3 and must NOT show P4
- [ ] Take screenshots; save under `screenshots/` in this task folder
- [ ] Confirm `docker compose -f docker-compose.dev.yaml logs api 2>&1 | grep "canonical org roles synced"` shows the startup sync ran exactly once

## Step 6 — Inspect production-shape data (diagnostic only)

- [ ] After the startup re-sync runs, query the inner DB to confirm the canonical roles in every org now have the up-to-date Config:
  ```bash
  docker exec helix-postgres-1 psql -U postgres -d postgres -c \
    "SELECT name, config FROM roles WHERE name IN ('read','write','admin') ORDER BY name LIMIT 30;"
  ```
- [ ] Note any orgs where the row is still drifted — that indicates the re-sync logic missed an edge case; investigate before merge
