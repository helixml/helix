# Requirements: Team-Based Project Visibility

## Problem Statement

An org member who belongs to a team that holds an access grant on a project cannot see that project in the org project list. Promoting the user to org owner makes the project visible (because org owners bypass per-project authorization in `listOrganizationProjects`).

Reported case: `https://app.helix.ml/orgs/the-linux-foundation`, team granted "admin" on a project, user is org member only.

## User Stories

**US-1:** As an org member who is in a team that has any access grant on a project, I see that project in the org project list.

**US-2:** As an org member with team-based access, I can open the project page and exercise the actions allowed by my team's role.

## Acceptance Criteria

- AC-1: Org member in a team with an `admin` access grant on a project sees the project in `GET /api/v1/projects?organization_id=...`.
- AC-2: Org member in a team with a `read` access grant on a project sees the project in the same listing.
- AC-3: Org member in a team with a `write` access grant on a project sees the project in the same listing.
- AC-4: Org owners and project owners continue to see all the projects they currently see (no regression).
- AC-5: A user not in any team and with no direct grant does NOT see the project (no regression — RBAC still enforced).

## Investigation Summary

### Confirmed Bug

**`RoleRead` and `RoleWrite` configs in `api/pkg/types/authz_roles.go` omit `ResourceProject`.** The `evaluate()` function in `api/pkg/server/authz.go:389` only matches when a rule's `Resources` list contains the requested resource OR `ResourceAny`. With `read` or `write` granted to a team on a project, the lookup returns the grant but the rule does not match `ResourceProject`, so the user fails authorization.

```go
// authz_roles.go (current)
RoleRead = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication, ResourceKnowledge, ResourceAccessGrants, // no ResourceProject
    }, ...
}}}
```

This alone explains the symptom for any team that holds a non-admin role on the project.

### Unconfirmed (admin role case)

The user's report mentions the team was an "admin" of the project. `RoleAdmin` uses `ResourceAny` (`"*"`), which `evaluate()` matches for any requested resource — so the admin path *should* work end-to-end. We have not reproduced this case from code analysis alone. Possible (but unverified) explanations:

- The "admin" the user is referring to in the UI may map to a role whose stored DB config differs from `types.RoleAdmin` (e.g., if the role config was edited, or seeded by an older version of the code).
- A separate data condition (soft-deleted team, missing `OrganizationID` on the `team_memberships` row, etc.).

The implementation must therefore include verification against the real `the-linux-foundation` data after the code fix lands, so we can confirm whether AC-1 is met by the read/write fix alone or if a second issue remains.

### Out of Scope

- Allowing non-admin team roles to perform write/delete on the project (we only fix visibility/`Get`).
- Reworking the `RoleAdmin` ResourceAny semantics.
