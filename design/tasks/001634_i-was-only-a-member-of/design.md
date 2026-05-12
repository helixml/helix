# Design: Team-based project visibility

## Authorization recap

Backend flow when a non-owner org member lists projects:

```
listProjects (project_handlers.go:32)
  → query has organization_id
    → listOrganizationProjects (project_handlers.go:70)
      → authorizeOrgMember(user, org)               // confirms membership
      → ListProjects(orgID)                          // ALL projects in org, unfiltered
      → IF orgMembership.Role == Owner → return all  // <-- the user got past this only after promotion
      → for each project:
          authorizeUserToProject(user, project, ActionGet)
            → IF user.ID == project.UserID → allow
            → IF orgMembership.Role == Owner → allow
            → authorizeUserToResource(user, org, project.ID, ResourceProject, ActionGet)
              → getAuthzConfigs(user, org, project.ID)
                  → ListTeams(orgID, userID)         // teams user belongs to
                  → ListAccessGrants(orgID, userID, projectID, teamIDs)
                  → return [role.Config for each grant.Roles]
              → evaluate(ResourceProject, ActionGet, configs)
                  → for each rule, match Resources contains ResourceProject OR ResourceAny
                    AND Actions contains ActionGet AND Effect=Allow
```

`evaluate()` is **allow-by-listing**: missing the requested resource in every rule means deny. There is no "everything except X" — only `ResourceAny` (`"*"`).

## Where each role lands today

| Role | Rules.Resources | Matches `ResourceProject`? |
|---|---|---|
| `RoleRead` | `[Application, Knowledge, AccessGrants]` | **No** |
| `RoleWrite` | `[Application, Knowledge]` | **No** |
| `RoleAdmin` | `[ResourceAny]` (`"*"`) | Yes (in code; possibly stale in DB) |

Source: `api/pkg/types/authz_roles.go` and `api/pkg/types/authz.go:301-319`.

## Two-part root cause

### Part A — code defect (definite)

`RoleRead` and `RoleWrite` don't list `ResourceProject`. Any team granted those roles on a project will not see the project. Reproducible by anyone, on any org, today.

### Part B — production-data drift (most likely explanation for the admin case)

Roles are seeded **once** per org by `seedOrganizationRoles` (`organization_handlers.go:337`):

```go
for _, role := range types.Roles {
    orgRole := &types.Role{
        ID:             system.GenerateRoleID(),
        OrganizationID: org.ID,
        Name:           role.Name,
        Description:    role.Description,
        Config:         role.Config,    // JSONB snapshot of the canonical at that moment
    }
    _, err := apiServer.Store.CreateRole(ctx, orgRole)
    ...
}
```

`ensureRoles` (`access_grant_handlers.go:193`) at grant-creation time looks up by `name` against this per-org snapshot, so the *DB-stored* `Config` is what `evaluate()` sees — not the in-code definition. There is no mechanism that updates existing rows when `types.Roles` changes.

If the canonical `RoleAdmin` was ever expanded (e.g. `ResourceAny` was added later, or a new action), older orgs keep the old config. `the-linux-foundation` is an old, large production org; the most plausible explanation for "admin role on a team can't see the project" given the code is correct is that its `roles.config` for `name='admin'` is missing `ResourceAny` (or has different actions).

We can confirm with one query (after the fix is deployed; we can also run it before to make the diagnosis concrete — see Verification §3):

```sql
SELECT r.name, r.config
  FROM roles r
  JOIN organizations o ON o.id = r.organization_id
 WHERE o.name = 'the-linux-foundation';
```

Both parts are fixed by this work. Part A by editing `types.Roles`. Part B by making seeding idempotent + re-running for all orgs at server start.

## Implementation

### Step 1 — `api/pkg/types/authz_roles.go`

Append `ResourceProject` to `RoleRead` and `RoleWrite`. `RoleAdmin` already covers it via `ResourceAny`.

```go
RoleRead = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication,
        ResourceKnowledge,
        ResourceAccessGrants,
        ResourceProject,             // NEW
    },
    Actions: []Action{ActionGet, ActionList, ActionUseAction},
    Effect:  EffectAllow,
}}}

RoleWrite = Config{Rules: []Rule{{
    Resources: []Resource{
        ResourceApplication,
        ResourceKnowledge,
        ResourceProject,             // NEW
    },
    Actions: []Action{ActionGet, ActionList, ActionUseAction, ActionCreate, ActionUpdate, ActionDelete},
    Effect:  EffectAllow,
}}}
```

Note: `RoleWrite` still doesn't grant `AccessGrants` (intentional — only admins manage grants), and we are NOT adding `Create`/`Update`/`Delete` for `Project` to `RoleWrite` (out of scope; this is a visibility fix). The added `Actions` apply to all listed `Resources`, so `RoleWrite` users could in principle delete a Project they have a grant on — that's not new behaviour for any other resource on this list and matches the existing semantics for `Application`/`Knowledge`. If the team feels that's too permissive for `Project` specifically, follow-up: split the rule.

### Step 2 — Make seeding idempotent + run for all orgs at startup

Two parts:

**2a.** Change `seedOrganizationRoles` to upsert by `(organization_id, name)`:

```go
func (apiServer *HelixAPIServer) seedOrganizationRoles(ctx context.Context, org *types.Organization) error {
    existing, err := apiServer.Store.ListRoles(ctx, org.ID)
    if err != nil {
        return fmt.Errorf("listing existing roles: %w", err)
    }
    byName := make(map[string]*types.Role, len(existing))
    for _, r := range existing {
        byName[r.Name] = r
    }
    for _, canonical := range types.Roles {
        if cur, ok := byName[canonical.Name]; ok {
            // Update Config and Description if drift detected.
            if !configsEqual(cur.Config, canonical.Config) || cur.Description != canonical.Description {
                cur.Config = canonical.Config
                cur.Description = canonical.Description
                if _, err := apiServer.Store.UpdateRole(ctx, cur); err != nil {
                    return fmt.Errorf("updating role %q: %w", canonical.Name, err)
                }
            }
            continue
        }
        // Insert if missing (existing behaviour).
        orgRole := &types.Role{
            ID:             system.GenerateRoleID(),
            OrganizationID: org.ID,
            Name:           canonical.Name,
            Description:    canonical.Description,
            Config:         canonical.Config,
        }
        if _, err := apiServer.Store.CreateRole(ctx, orgRole); err != nil {
            return fmt.Errorf("creating role %q: %w", canonical.Name, err)
        }
    }
    return nil
}
```

`configsEqual` is a JSON-equality helper (marshal both and compare bytes — they go through the same `Config.Value`/`Scan` round-trip anyway). If `UpdateRole` doesn't already exist on the store, add it (mirrors `CreateRole`).

**2b.** Call it for every existing org during server startup. Best landing spot is the existing init path in `api/pkg/server/server.go` next to other one-shot initialisations. Pseudocode:

```go
// After Store and route setup, before serve.
orgs, err := s.Store.ListOrganizations(ctx, &store.ListOrganizationsQuery{})
if err != nil {
    return fmt.Errorf("loading orgs for role re-sync: %w", err)
}
updated := 0
for _, o := range orgs {
    if err := s.seedOrganizationRoles(ctx, o); err != nil {
        log.Err(err).Str("org_id", o.ID).Msg("role re-sync failed")
        // Don't fail startup for one bad org; log and continue.
        continue
    }
    updated++
}
log.Info().Int("orgs", updated).Msg("canonical org roles synced at startup")
```

Custom roles (any name not in `types.Roles`) are left untouched. Idempotent — re-running is a no-op once configs match.

### Step 3 — Tests in `api/pkg/server/authz_test.go`

Add a project-via-team suite paralleling the existing `AuthzAppSuite`. Use the existing `gomock`/`MockStore` pattern. The four cases:

1. `TestProjectViaTeam_AdminAllowed` — team has admin grant → `authorizeUserToProject(ActionGet)` returns nil.
2. `TestProjectViaTeam_ReadAllowed` — team has read grant.
3. `TestProjectViaTeam_WriteAllowed` — team has write grant.
4. `TestProjectViaTeam_NoGrantDenied` — team exists, user is in it, but no grant on this project → returns error.

Mocks must:
- Return a non-empty team ID from `ListTeams` (regression-guard against the empty-team-ID class of bug);
- Return a grant from `ListAccessGrants` whose `Roles[].Config` equals the canonical Config under test (re-using `types.RoleAdmin`/`RoleRead`/`RoleWrite` directly is fine and ties the tests to the canonical definitions — if someone later removes `ResourceProject` from the canonical, these tests break loudly).

Optional but cheap: parameterise as a table test. Pattern:

```go
cases := []struct{ name string; cfg types.Config; wantErr bool }{
    {"admin", types.RoleAdmin, false},
    {"read",  types.RoleRead,  false},
    {"write", types.RoleWrite, false},
}
```

Plus a separate test for the negative case (no grants returned).

### Step 4 — A test for the upsert-seed behaviour

Add to `organization_handlers_test.go` (create if absent — mirror the existing test files in `pkg/server/`):

- Seed an org with the canonical roles.
- Mutate the DB-stored `Config` for the `admin` role (simulate drift).
- Call `seedOrganizationRoles` again.
- Assert the row's `Config` now equals `types.RoleAdmin`.

This guards against the seeding accidentally going back to insert-only.

## Files touched

| File | Change |
|---|---|
| `api/pkg/types/authz_roles.go` | Append `ResourceProject` to `RoleRead`, `RoleWrite` |
| `api/pkg/server/organization_handlers.go` | `seedOrganizationRoles` becomes upsert |
| `api/pkg/server/server.go` (or wherever startup init lives — verify) | Call `seedOrganizationRoles` for every org at boot |
| `api/pkg/store/roles.go` (or wherever `CreateRole` lives — verify) | Add `UpdateRole` if missing |
| `api/pkg/server/authz_test.go` | Add 4 tests for project-via-team access |
| `api/pkg/server/organization_handlers_test.go` | Add upsert-drift test |

## Verification

1. `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
2. `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1` — new tests green.
3. **(Optional, do this BEFORE the fix to make the diagnosis concrete.)** Query the inner Helix DB to inspect the actual stored config for the canonical roles in the `the-linux-foundation` org, if it exists locally; or simulate it by inserting a deliberately-broken admin row in a fresh org and confirming the team-admin user can't see the project pre-fix:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT name, config FROM roles WHERE name IN ('read','write','admin') ORDER BY name LIMIT 20;"
   ```
4. End-to-end in inner Helix at `http://localhost:8080`:
   - Register `test@helix.local` / `testpass123`, complete onboarding, create an org.
   - Register a second user (e.g. `member@helix.local`); add as **member** of the org.
   - Create a team in the org; add the second user to the team.
   - Create a project in the org; grant the team `admin` on the project.
   - Log in as the second user → org page must list the project.
   - Repeat with `read` and `write` grants on a fresh project each.
5. Negative test in the same env: create another project with no grants → second user must NOT see it.

## Notes for future agents

- `evaluate()` is allow-by-listing — adding a new `Resource` constant requires updating every canonical role config that should grant it. There is no "all known resources except X" wildcard. If a future bug looks like "RBAC silently denies on a new resource", check `authz_roles.go` first.
- Org owners bypass per-project authz at `project_handlers.go:90`. That's why promoting to owner papered over this bug. Don't remove that bypass.
- Roles are stored per-org (`api/pkg/types/authz.go:Role`) as JSONB Config + a name. `ensureRoles` looks up by name when granting, so the role's *current* DB-stored config is what gets enforced — code-level edits to `types.Roles` do not propagate without a re-sync. Step 2 of this fix establishes the re-sync; future canonical changes are now safe.
- GORM `Preload("Team")` / `Preload("User")` on `TeamMembership` works via convention (no `gorm:"foreignKey:..."` tag required). The same pattern is used in `OrganizationMembership` and works in production. Don't waste time chasing a missing-tag theory.
- Frontend `useListProjects(orgID)` passes the actual org ID (from the loaded `Organization` object), not the slug. The slug → ID resolution happens earlier in `useOrganizations`. If a similar bug points at `lookupOrg`, that's a different code path.
- The `AccessGrant` row alone does NOT carry the role config; it carries role IDs via `AccessGrantRoleBinding`. Each `Role` row holds the actual `Config`. So fixing role configs in the DB fixes already-existing grants without any access-grant table updates.
