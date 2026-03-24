# Requirements: Team-Based Project Visibility Bug

## Problem Statement

An org member who belongs to a team that has been granted admin access to a project cannot see that project in the org project list. Only after being promoted to org owner does the project become visible. This violates the expected RBAC behavior.

## User Stories

**US-1:** As an org member who belongs to a team with any role (read/write/admin) on a project, I should be able to see that project when listing org projects.

**US-2:** As an org member with team-based access to a project, I should be able to open and interact with that project according to the role granted to my team.

## Acceptance Criteria

- AC-1: A user who is a member of a team that has an `admin` access grant on a project can see and access the project when listing org projects.
- AC-2: A user who is a member of a team that has a `read` access grant on a project can see the project when listing org projects.
- AC-3: A user who is a member of a team that has a `write` access grant on a project can see the project when listing org projects.
- AC-4: Existing orgs whose seeded roles predate any fix still work correctly (i.e., the fix handles stale DB role configs).
- AC-5: Org owners continue to see all projects (no regression).
- AC-6: Project owners continue to see their own projects regardless of team membership (no regression).

## Root Cause Analysis

Two related bugs found in `api/pkg/types/authz_roles.go`:

**Bug 1 — RoleRead/RoleWrite missing `ResourceProject`:**
`RoleRead` and `RoleWrite` enumerate specific resource types (`ResourceApplication`, `ResourceKnowledge`, `ResourceAccessGrants`) but do NOT include `ResourceProject`. The `evaluate()` function in `authz.go` checks resources explicitly, so team members granted `read` or `write` on a project fail the authorization check and cannot see the project.

```go
// authz_roles.go - current (broken)
RoleRead = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication,
        ResourceKnowledge,
        ResourceAccessGrants,  // missing ResourceProject
    }, ...
}}}
```

**Bug 2 — Stale role configs in existing orgs:**
Roles are seeded into the DB when an org is created (`seedOrganizationRoles`). If `RoleRead`/`RoleWrite` configs are updated in code, existing orgs retain the old DB-stored configs. There is no mechanism to sync role configs when the canonical definitions change.

**Note on RoleAdmin:** `RoleAdmin` uses `ResourceAny` (`"*"`), which correctly matches `ResourceProject` via `resource == types.ResourceAny` in `evaluate()`. So team members with the `admin` role should theoretically see the project — however, the user reported that even `admin` did not work, which suggests the stale DB config issue (Bug 2) may be the primary cause, or there is an additional issue with `ListTeams` not returning teams when the user is a member.
