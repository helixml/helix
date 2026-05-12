# Design: Team-Based Project Visibility Fix

## Code Map

| File | Role |
|------|------|
| `api/pkg/server/project_handlers.go:70` (`listOrganizationProjects`) | Lists org projects; for non-owners filters via `authorizeUserToProject` |
| `api/pkg/server/authz.go:202` (`authorizeUserToProject`) | Calls `authorizeUserToResource(... ResourceProject ...)` for non-owner, non-creator |
| `api/pkg/server/authz.go:352` (`getAuthzConfigs`) | `ListTeams(orgID, userID)` → `ListAccessGrants(orgID, userID, projectID, teamIDs)` |
| `api/pkg/server/authz.go:389` (`evaluate`) | Returns true iff a rule matches `requestedResource` or `ResourceAny` |
| `api/pkg/types/authz_roles.go` | Canonical `RoleRead` / `RoleWrite` / `RoleAdmin` configs (seeded into DB at org creation) |
| `api/pkg/server/organization_handlers.go:337` (`seedOrganizationRoles`) | Inserts the canonical roles when an org is created |
| `api/pkg/store/teams.go:86` (`ListTeams`) | When `UserID` set, joins through `team_memberships`, returns `membership.Team` (preloaded) |
| `api/pkg/store/access_grant.go:99` (`ListAccessGrants`) | Filters by `org`, `resource_id`, and `(user_id = ? OR team_id IN (?))` |

## Fix 1 — Add `ResourceProject` to `RoleRead` and `RoleWrite`

In `api/pkg/types/authz_roles.go`, add `ResourceProject` to both role configs.

```go
RoleRead = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication,
        ResourceKnowledge,
        ResourceAccessGrants,
        ResourceProject,
    },
    Actions: []Action{ActionGet, ActionList, ActionUseAction},
    Effect:  EffectAllow,
}}}

RoleWrite = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication,
        ResourceKnowledge,
        ResourceProject,
    },
    Actions: []Action{ActionGet, ActionList, ActionUseAction, ActionCreate, ActionUpdate, ActionDelete},
    Effect:  EffectAllow,
}}}
```

`RoleAdmin` already uses `ResourceAny` and needs no edit at the type-definition level.

## Fix 2 — Re-sync Seeded Role Configs in Existing Orgs

`seedOrganizationRoles` only runs at org creation. When `types.Roles` changes, existing orgs keep the older configs in the `roles` table. We need a one-shot, idempotent re-sync so production orgs (including `the-linux-foundation`) pick up the corrected `RoleRead`/`RoleWrite` definitions without requiring re-creation.

Add a startup-time function (called once during server boot, similar to other init paths):

```go
// In organization_handlers.go (or a new file).
// For each org × each canonical role:
//   - If a role with the same name exists, UPDATE its Config to match the canonical config.
//   - If absent, INSERT it.
// Match strictly by (organization_id, name). Custom roles (no name match) are untouched.
func (s *HelixAPIServer) syncCanonicalOrgRoles(ctx context.Context) error { ... }
```

Wire `syncCanonicalOrgRoles` into the server startup sequence (next to other one-shot init code in `server.go`). On boot it walks `ListOrganizations` and calls `Store.UpdateRole` (or `CreateRole` if missing) for each canonical role.

This is non-destructive (only touches roles whose `name` matches a canonical role) and idempotent (running twice is a no-op).

## Fix 3 — Tests

Add to `api/pkg/server/authz_test.go` (existing pattern with `gomock` + `MockStore`) — these are the missing tests that would have caught the bug:

1. **TestProjectAccessViaTeamWithRead** — user in a team with a `read` grant on a project: `authorizeUserToProject(ActionGet)` returns nil.
2. **TestProjectAccessViaTeamWithWrite** — same as above with `write`.
3. **TestProjectAccessViaTeamWithAdmin** — same as above with `admin` (covers `ResourceAny` path).
4. **TestProjectAccessViaTeamNoGrant** — user is in a team but team has no grant on this project → returns error.

The mocks should set up `ListTeams` to return a real team (non-empty ID) and `ListAccessGrants` to return a grant with the role config under test, mirroring how production code wires the lookup.

## Verification (After Implementation)

Per `helix-4/CLAUDE.md` ("test every change"):

1. Build: `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`.
2. Unit: `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1`.
3. End-to-end in inner Helix at `http://localhost:8080`:
   - Register `test@helix.local` / `testpass123`, complete onboarding.
   - As owner: create a second user, add as org member (not owner). Create team. Add member to team. Create a project. Grant team admin on project.
   - Log in as the second user → org page must list the project.
   - Repeat with `read` and `write` grants.

If the inner-Helix verification still shows the project hidden for the admin-role case, the second issue is in data shape — not code logic — and the next debugging step is to query Postgres directly:

```sql
-- Replace ORG_NAME accordingly
SELECT r.name, r.config FROM roles r
  JOIN organizations o ON o.id = r.organization_id
 WHERE o.name = 'the-linux-foundation';

SELECT ag.id, ag.resource_id, ag.team_id, ag.user_id, agr.role_id, r.name, r.config
  FROM access_grants ag
  LEFT JOIN access_grant_role_bindings agr ON agr.access_grant_id = ag.id
  LEFT JOIN roles r ON r.id = agr.role_id
 WHERE ag.organization_id = (SELECT id FROM organizations WHERE name = 'the-linux-foundation');
```

That tells us whether the admin role's stored `config` is the expected JSONB.

## Decisions and Tradeoffs

- **Sync at startup vs. SQL migration.** Project uses GORM `AutoMigrate` for schema only; data updates are done in Go (`seedOrganizationRoles` is the precedent). A startup sync keeps the pattern consistent and is idempotent.
- **Update only matching role names.** We do not touch custom roles or roles whose name doesn't match a canonical name — narrowest blast radius.
- **Don't broaden `ResourceAny` matching or change `RoleAdmin`.** Tempting but unrelated; would expand scope and risk regressions in App/Knowledge access checks.

## Codebase Notes for Future Work

- `OrganizationMembership.User`, `TeamMembership.Team`, `TeamMembership.User` rely on GORM convention (no explicit `gorm:"foreignKey:..."` tag) — the convention DOES work for `belongs to` when the foreign key field name follows the `<RelName>ID` pattern; widely-used Preloads elsewhere in the codebase confirm this.
- The `Config` type (`api/pkg/types/authz.go`) is JSONB with custom `Scan`/`Value` — round-trips through Postgres preserve the rule resources/actions.
- `evaluate()` is allow-by-listing: missing a resource in a rule means deny for that resource. This makes `RoleRead`/`RoleWrite` brittle whenever a new `Resource` is introduced; consider adding a CI check or comment that flags this for future resource additions.
