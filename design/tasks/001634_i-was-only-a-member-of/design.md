# Design: Team-based project visibility

## Authorization flow (non-owner org member listing projects)

```
listProjects (project_handlers.go:32)
  → query has organization_id
    → listOrganizationProjects (project_handlers.go:70)
      → authorizeOrgMember(user, org)
      → ListProjects(orgID)                          // ALL projects, unfiltered
      → IF orgMembership.Role == Owner → return all  // <-- the bypass that "fixed" it via promotion
      → for each project:
          authorizeUserToProject(user, project, ActionGet)
            → IF user.ID == project.UserID → allow
            → IF orgMembership.Role == Owner → allow
            → authorizeUserToResource(... ResourceProject, ActionGet)
              → getAuthzConfigs → ListTeams + ListAccessGrants
              → evaluate(ResourceProject, ActionGet, configs)
                  → match Resources contains ResourceProject OR ResourceAny
                    AND Actions contains ActionGet AND Effect=Allow
```

`evaluate()` is allow-by-listing: missing the resource means deny. Only `ResourceAny` (`"*"`) wildcards.

## Where each canonical role lands

| Role | Resources (before fix) | Matches `ResourceProject`? |
|---|---|---|
| `RoleRead` | `[Application, Knowledge, AccessGrants]` | **No** |
| `RoleWrite` | `[Application, Knowledge]` | **No** |
| `RoleAdmin` | `[ResourceAny]` (`"*"`) | Yes |

Source: `api/pkg/types/authz_roles.go`.

## Root cause

`RoleRead` and `RoleWrite` omit `ResourceProject`. Any team granted those roles on a project loads the grant correctly but fails the rule match → user does not see the project. Reproducible for any org.

## On the admin-role part of the user's report

The user reported the team had the **admin** role. With the code as-is, `RoleAdmin` uses `ResourceAny` and should match `ResourceProject`. I initially assumed the production org might have a stale `roles.config` JSONB from older code — but **that's already handled**: `autoMigrateRoleConfig` in `api/pkg/store/organization_roles.go:12` runs at every server startup (called from `postgres.go:228` right after `AutoMigrate`) and updates all role rows whose `name` matches a canonical role with the current `types.Roles` config. So `the-linux-foundation`'s admin role config is in sync.

Best explanation: the user's team likely had `read` (the default for grants in some UI flows) or `write`, not `admin`. Either way, the read/write fix covers the reported symptom — the team admin path needed no code change and the new test pins it as a regression guard.

## Fix

**Single change**: append `ResourceProject` to `RoleRead.Rules[0].Resources` and `RoleWrite.Rules[0].Resources` in `api/pkg/types/authz_roles.go`. `autoMigrateRoleConfig` propagates the new config to all existing orgs on next API server boot — no migration, no startup-sync code needed.

## Tests

`api/pkg/server/authz_test.go` gains `AuthzProjectViaTeamSuite` (4 cases) using the existing `gomock`/`MockStore` pattern:

1. `TestTeamAdminGrant_AllowsProjectGet` — regression guard for the `ResourceAny` path.
2. `TestTeamReadGrant_AllowsProjectGet` — bug-fix coverage for `RoleRead`.
3. `TestTeamWriteGrant_AllowsProjectGet` — bug-fix coverage for `RoleWrite`.
4. `TestTeamMembership_NoGrant_Denied` — negative case (user in a team with no grant on the project).

Mocks use `types.RoleAdmin`/`RoleRead`/`RoleWrite` directly so future removals of `ResourceProject` from the canonical fail the tests loudly. `ListTeams` mock returns a non-empty team ID (regression guard against empty-team-ID class of bug).

## Files touched

| File | Change |
|---|---|
| `api/pkg/types/authz_roles.go` | Append `ResourceProject` to `RoleRead`, `RoleWrite` |
| `api/pkg/server/authz_test.go` | Add `AuthzProjectViaTeamSuite` with 4 tests |

## Verification

- `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` — passes.
- `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1` — all green, including 4 new tests, no existing test regressions.

## Notes for future agents

- **`evaluate()` is allow-by-listing.** Adding a new `Resource` constant means every canonical role config that should grant it needs the resource added. There's no "all known resources except X" wildcard — only `ResourceAny`.
- **Org owners bypass per-project authz** at `project_handlers.go:90`. That's why promoting to owner "fixed" this bug. Don't remove that bypass.
- **Role configs are kept in sync at startup** by `autoMigrateRoleConfig` (`api/pkg/store/organization_roles.go:12`, called from `postgres.go:228`). Don't add a parallel sync mechanism — extend that one.
- **GORM `Preload("Team")` / `Preload("User")` on `TeamMembership`** works via convention (no `gorm:"foreignKey:..."` tag). Same pattern in `OrganizationMembership` and used in production. Don't chase a missing-tag theory.
- **Frontend `useListProjects(orgID)`** passes the actual org ID (resolved from slug earlier in `useOrganizations`). Slug routing is not in the request path for the project list.
- **`AccessGrant` carries role IDs via `AccessGrantRoleBinding`**, not role configs. So fixing role configs in the `roles` table fixes already-existing grants automatically.
