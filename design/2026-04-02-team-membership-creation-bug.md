# Team Membership Creation Bug -- Investigation and Fix Plan

## Summary

- Team creation does NOT auto-add the creator as a team member
- The `/teams/{id}/access-grants` API endpoints do not exist
- Authorization fails for newly created teams because the creator has no membership record

## Bug Reproduction

1. Log in as an org owner
2. Create a team via UI or `POST /api/v1/organizations/{org_id}/teams`
3. Observe: "user is not authorized to perform this action" on subsequent team operations
4. API logs show `error getting team membership: not found` at `organization_team_handlers.go:295`
5. `POST .../teams/{team_id}/access-grants` returns "unknown API path"

## Root Cause

### Missing auto-membership on team creation

`api/pkg/server/organization_team_handlers.go` `createTeam()` (lines 48-93) creates the team
record but never calls `CreateTeamMembership()`. Compare with `organization_handlers.go` lines
315-319 where org creation auto-adds the creator as owner:

```go
// organization_handlers.go -- correct pattern
_, err = apiServer.Store.CreateOrganizationMembership(ctx, &types.OrganizationMembership{
    OrganizationID: createdOrg.ID,
    UserID:         user.ID,
    Role:           types.OrganizationRoleOwner,
})
```

The equivalent call is missing from `createTeam()`.

### Missing access-grants endpoints

Routes registered in `server.go` lines 985-991:

```
GET    /organizations/{id}/teams                              -- exists
POST   /organizations/{id}/teams                              -- exists
PUT    /organizations/{id}/teams/{team_id}                    -- exists
DELETE /organizations/{id}/teams/{team_id}                    -- exists
GET    /organizations/{id}/teams/{team_id}/members            -- exists
POST   /organizations/{id}/teams/{team_id}/members            -- exists
DELETE /organizations/{id}/teams/{team_id}/members/{user_id}  -- exists
GET    /organizations/{id}/teams/{team_id}/access-grants      -- MISSING
POST   /organizations/{id}/teams/{team_id}/access-grants      -- MISSING
DELETE /organizations/{id}/teams/{team_id}/access-grants/{id} -- MISSING
```

Access-grants endpoints exist for apps (lines 858-860), projects (lines 1210-1212), and
repositories (lines 1326-1328) but were never added for teams. The store layer already supports
team access grants -- `types.AccessGrant` has a `TeamID` field and `store.ListAccessGrants()`
accepts `TeamIDs`.

### Authorization chain failure

`api/pkg/server/authz.go` lines 352-387 (`getAuthzConfigs()`) queries teams by user membership:

```go
teams, err := db.ListTeams(ctx, &store.ListTeamsQuery{
    OrganizationID: orgID,
    UserID:         user.ID,  // filters by membership
})
```

No membership record -> empty teams list -> no access grants found -> authorization fails.

## Data Model

```
Organization (1) --- (N) OrganizationMembership --- User
    |
    +-- (N) Team
              |
              +-- (N) TeamMembership --- User

AccessGrant can reference:
  - UserID           (direct user grant)
  - TeamID           (team-based grant)
  - OrganizationID   (org-wide grant)
```

Database schema and FK constraints are correct (store/postgres.go lines 245-249). CASCADE
deletes work properly. No schema changes needed.

## Fix Plan

### P0: Auto-add creator as team member

File: `api/pkg/server/organization_team_handlers.go`, `createTeam()`

After the `CreateTeam()` call (line 85), add:

```go
_, err = apiServer.Store.CreateTeamMembership(r.Context(), &types.TeamMembership{
    TeamID:         createdTeam.ID,
    UserID:         user.ID,
    OrganizationID: orgID,
})
if err != nil {
    log.Err(err).Msg("error adding creator to team")
    http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
    return
}
```

### P1: Implement team access-grants endpoints

File: `api/pkg/server/server.go` -- add route registrations
File: `api/pkg/server/access_grant_handlers.go` -- add handlers (follow existing app pattern)

Routes to add:

```go
authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/access-grants",
    apiServer.listTeamAccessGrants).Methods(http.MethodGet)
authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/access-grants",
    apiServer.createTeamAccessGrant).Methods(http.MethodPost)
authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/access-grants/{grant_id}",
    apiServer.deleteTeamAccessGrant).Methods(http.MethodDelete)
```

The store already supports this -- `ListAccessGrants` accepts `TeamIDs` and `AccessGrant` has
a `TeamID` field. Only handler and route wiring is needed.

### P2: Integration tests

File: `api/pkg/server/organizations_rbac_test.go` or new test file

Test cases:
1. Team creator is automatically a member after creation
2. Team creator can list and manage the team
3. Team access grants can be created, listed, deleted
4. Non-team-members cannot access team resources
5. Deleting a team cascades to memberships

## Key Files

| File | What to change |
|------|----------------|
| `api/pkg/server/organization_team_handlers.go` | Add CreateTeamMembership in createTeam() |
| `api/pkg/server/server.go:985-991` | Add access-grants route registrations |
| `api/pkg/server/access_grant_handlers.go` | Add team access-grant handlers |
| `api/pkg/server/authz.go:352-387` | No change needed (works once membership exists) |
| `api/pkg/store/team_membership.go` | No change needed (already implemented) |
| `api/pkg/store/teams.go` | No change needed |
| `api/pkg/types/authz.go` | No change needed (TeamID on AccessGrant exists) |

## Why This Bug Exists

1. Organization creation followed the auto-add pattern; team creation did not
2. RBAC integration tests create teams via org owners who bypass team-level checks
3. Access-grants were implemented for apps/projects/repos but never wired for teams
4. The store and type layers are complete -- only the handler layer has gaps

## Observed Errors (from API logs, 2026-04-02)

```
15:48:06 ERR error for route: user is not authorized to perform this action
15:48:21 ERR error getting team membership: not found
         (organization_team_handlers.go:295)
15:48:21 ERR unknown API path method=POST
         path=/api/v1/organizations/.../teams/.../access-grants
```
