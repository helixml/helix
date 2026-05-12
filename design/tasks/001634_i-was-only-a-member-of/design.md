# Design

## Root cause

`RoleRead` and `RoleWrite` in `api/pkg/types/authz_roles.go` enumerate `ResourceApplication`, `ResourceKnowledge`, `ResourceAccessGrants` — but **omit `ResourceProject`**. `evaluate()` in `api/pkg/server/authz.go` is allow-by-listing: a rule only grants access when its `Resources` list contains the requested resource OR `ResourceAny`. So a team granted `read`/`write` on a project loads the grant correctly but fails the rule match → user does not see the project.

`RoleAdmin` uses `ResourceAny` and *should* work end-to-end. The user reports it doesn't for `the-linux-foundation`. Most likely: that org's row in the `roles` table has a stale JSONB `config` from when the canonical role definition was different. Roles are seeded **once** on org creation by `seedOrganizationRoles` (`api/pkg/server/organization_handlers.go:337`) and never re-synced when `types.Roles` changes.

## Fix

1. In `api/pkg/types/authz_roles.go`, append `ResourceProject` to `RoleRead.Rules[0].Resources` and `RoleWrite.Rules[0].Resources`.

2. Make role seeding upsert + run for all orgs at server boot:
   - Change `seedOrganizationRoles` so for each canonical role: if a row with the same `(organization_id, name)` exists, UPDATE its `config`/`description` to match the canonical version; else INSERT.
   - Add a startup pass in the server init (next to other one-shot init in `server.go`) that iterates `Store.ListOrganizations` and calls the upsert. Custom roles (no name match) are untouched. Idempotent.

3. Add `gomock`-based tests in `api/pkg/server/authz_test.go` mirroring the existing `AuthzRepositorySuite` / `AuthzAppSuite` style: org member in a team with a `read` / `write` / `admin` grant on a project gets `nil` from `authorizeUserToProject(ActionGet)`; org member in a team with no grant gets an error.

## Verification

- `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
- `CGO_ENABLED=1 go test -v -run TestAuthz ./pkg/server/ -count=1`
- E2E in inner Helix at `http://localhost:8080`: register → create org → add 2nd user as member → create team → add 2nd user → create project → grant team `admin` → log in as 2nd user → project must appear on org page. Repeat for `read` and `write`.

## Notes for future agents

- The matcher (`evaluate`) is allow-by-listing — adding a new `Resource` constant requires updating every canonical role config that should grant it. There is no "all known resources except X" wildcard; only `ResourceAny` (`"*"`).
- Org owners bypass per-project authz in `listOrganizationProjects` (`project_handlers.go:90`) — that's why promoting to owner papered over this bug.
- Roles are stored per-org (`api/pkg/types/authz.go:Role`) as JSONB Config + a name. `ensureRoles` looks up by name when granting, so the role's *current* DB-stored config is what gets enforced — code-level edits to `types.Roles` do not propagate without a re-sync.
- GORM `Preload("Team")` / `Preload("User")` on `TeamMembership` works via convention (no `gorm:"foreignKey:..."` tag needed); same pattern works in `OrganizationMembership` and is used in production. Don't waste time chasing a missing-tag theory.
- Frontend hits `GET /api/v1/projects?organization_id=<org_id>` (the actual ID, not the slug — frontend resolves slug to ID first via `GET /api/v1/organizations/<slug>`).
