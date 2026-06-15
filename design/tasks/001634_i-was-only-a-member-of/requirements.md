# Requirements: Team-based project visibility broken for org members

## Bug report

> "I was only a member of https://app.helix.ml/orgs/the-linux-foundation but had been added to a team that was added as an admin of a project, yet I could not see that project. Making me an owner of the org allowed me to see the project. But being part of a team that is admin on the project should have made the project visible to me."

Expectation: **org member + team grant on project = project visible**. Today: hidden until promoted to org owner.

## Affected surface

- `GET /api/v1/projects?organization_id=<org_id>` — backs the org page (`/orgs/:slug`).
- Frontend resolves slug → ID earlier; the project-list call uses the ID.

## Root cause

`RoleRead` and `RoleWrite` in `api/pkg/types/authz_roles.go` omit `ResourceProject`. The matcher in `evaluate()` (`api/pkg/server/authz.go:389`) only allows when the rule's `Resources` contains the requested resource OR `ResourceAny`. So a team granted `read`/`write` on a project loads the grant but fails the rule match.

`RoleAdmin` uses `ResourceAny` and matches `ResourceProject`. `autoMigrateRoleConfig` (`api/pkg/store/organization_roles.go:12`, called from `postgres.go:228` after `AutoMigrate`) syncs `roles.config` from `types.Roles` at every server start, so DB-stored admin configs cannot drift. The user's admin-role description is likely either a `read`/`write` grant they remembered as "admin", or a UI-labeling mismatch — the read/write fix covers the reported symptom either way.

## User stories

- **As an org member in a team that holds an access grant on a project**, I see that project in the org's project list and can open it.

## Acceptance criteria

1. Team `admin` grant → org member sees the project.
2. Team `read` grant → org member sees the project.
3. Team `write` grant → org member sees the project.
4. Org member with no team grant and no direct grant still does NOT see the project (negative).
5. Org owners still see all projects (no regression).
6. Project owners still see their own projects (no regression).
7. Tests pin AC 1–4 in `authz_test.go` so this regression class is visible to CI.

## Out of scope

- Whether non-admin team roles should be allowed to write/delete a project (only fixing `ActionGet` visibility).
- Frontend changes — bug is server-side; frontend renders whatever the API returns.
- Reworking per-org role storage to be code-only (custom roles also live in that table; we let `autoMigrateRoleConfig` handle the canonical ones).
