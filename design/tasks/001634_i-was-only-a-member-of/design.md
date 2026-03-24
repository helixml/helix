# Design: Team-Based Project Visibility Bug Fix

## Code Paths

**Project listing:** `api/pkg/server/project_handlers.go:listOrganizationProjects`
- Fetches all org projects, returns all for org owners, filters the rest via `authorizeUserToProject`

**Per-project authorization:** `api/pkg/server/authz.go:authorizeUserToProject`
- Falls through to `authorizeUserToResource` for non-owners and non-project-creators

**RBAC check:** `api/pkg/server/authz.go:getAuthzConfigs`
- Calls `store.ListTeams(orgID, userID)` to find the user's teams
- Calls `store.ListAccessGrants(orgID, userID, resourceID, teamIDs)` to find grants

**Role configs:** `api/pkg/types/authz_roles.go`
- `RoleRead`, `RoleWrite`, `RoleAdmin` — canonical in-code definitions
- Seeded into the DB per-org on org creation (`seedOrganizationRoles`)

**Role evaluation:** `api/pkg/server/authz.go:evaluate`
- Matches `resource == requestedResource || resource == types.ResourceAny`

## Fix 1: Add ResourceProject to RoleRead and RoleWrite

In `api/pkg/types/authz_roles.go`, add `ResourceProject` to `RoleRead` and `RoleWrite`:

```go
RoleRead = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication,
        ResourceKnowledge,
        ResourceAccessGrants,
        ResourceProject,  // add this
    },
    Actions: []Action{ActionGet, ActionList, ActionUseAction},
    Effect:  EffectAllow,
}}}

RoleWrite = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication,
        ResourceKnowledge,
        ResourceProject,  // add this
    },
    Actions: []Action{ActionGet, ActionList, ActionUseAction, ActionCreate, ActionUpdate, ActionDelete},
    Effect:  EffectAllow,
}}}
```

`RoleAdmin` already uses `ResourceAny` (`"*"`) which matches all resources including `ResourceProject` — no change needed.

## Fix 2: Sync Stale Role Configs on Startup

Roles are stored in the DB at org creation time. When the canonical configs in `authz_roles.go` change, existing orgs retain stale configs. Fix: on server startup (or as a one-time migration), update all org roles whose name matches a canonical role to have the latest canonical config.

Add a `syncOrganizationRoles(ctx, org)` function called from `CreateOrganization` AND from a startup migration that iterates all orgs and updates existing roles:

```go
// For each org, for each role in types.Roles:
//   if org already has a role with that name, update its Config
//   if org doesn't have the role, create it
func (apiServer *HelixAPIServer) syncOrganizationRoles(ctx context.Context, orgID string) error {
    // upsert canonical role configs for this org
}
```

This is the safest approach: non-destructive (updates matching by name only), idempotent (can run multiple times), and doesn't require a separate DB migration file.

## Fix 3: Add Missing Tests

Add tests to `api/pkg/server/authz_test.go` (or a new `project_handlers_test.go`) that cover:

1. **Team member with admin grant sees project** — `listOrganizationProjects` returns the project when the user is in a team that has an admin access grant on it.
2. **Team member with read grant sees project** — same as above but with read role.
3. **Team member with no grant doesn't see project** — user is in a team but the team has no grant on the project.

Use the existing mock-based test pattern (`gomock`, `MockStore`).

## Key Files

| File | Change |
|------|--------|
| `api/pkg/types/authz_roles.go` | Add `ResourceProject` to `RoleRead` and `RoleWrite` |
| `api/pkg/server/organization_handlers.go` | Add `syncOrganizationRoles` call; add startup sync |
| `api/pkg/server/authz_test.go` | Add team-based project access tests |

## Decision: Startup Sync vs. DB Migration

Chose startup sync over a SQL migration file because:
- The project uses GORM AutoMigrate (schema only, no data migrations infrastructure)
- Startup sync is already the pattern used for seeding (see `seedOrganizationRoles`)
- Idempotent — safe to run on every deploy

## Codebase Patterns Discovered

- Roles are stored as JSONB (`Config` type with custom `Scan`/`Value`) in the `roles` table, one row per role per org.
- `authorizeUserToProject` calls `authorizeOrgMember` internally (redundant call when called from `listOrganizationProjects` which already checked, but not a bug).
- `ListTeams` with `UserID` returns teams via a join through `TeamMembership` table; if a team is soft-deleted, the GORM preload would silently fail and return an empty `Team{}` (potential separate bug, but unlikely in normal operation).
- Standard role names: `"read"`, `"write"`, `"admin"`.
