# Requirements: Team-based project visibility broken for org members

## Bug report

> "I was only a member of https://app.helix.ml/orgs/the-linux-foundation but had been added to a team that was added as an admin of a project, yet I could not see that project. Making me an owner of the org allowed me to see the project. But being part of a team that is admin on the project should have made the project visible to me."

The user expects: **org member + team grant on project = project visible**. Today: project is hidden until the user is promoted to org owner (which bypasses per-project authz entirely).

## Affected surface

- `GET /api/v1/projects?organization_id=<org_id>` — the API call the org page (`/orgs/:org_id`) makes via `useListProjects(org.id)` in `frontend/src/services/projectService.ts:19`.
- Frontend hits the endpoint with the org's *ID* (not slug); the slug → ID resolution happens earlier via `GET /api/v1/organizations/:slug`. Slug routing is not the bug.

## Root cause (one-line)

`RoleRead` and `RoleWrite` in `api/pkg/types/authz_roles.go` enumerate the resources they grant access to but **omit `ResourceProject`**. The matcher in `evaluate()` (`api/pkg/server/authz.go:389`) only allows when the rule's `Resources` contains the requested resource OR `ResourceAny`. So a team granted `read`/`write` on a project loads the access-grant correctly but fails the rule match → user does not see the project.

`RoleAdmin` uses `ResourceAny` and *should* match `ResourceProject` end-to-end. The user's report says the team had the **admin** role and still couldn't see the project — that points to a second issue (likely stale per-org role config in the DB; see "Why admin also failed" below).

## Why admin also failed (most likely)

Roles are stored **per-org** in the `roles` table as JSONB Config + a name. They are seeded once at org creation by `seedOrganizationRoles` (`api/pkg/server/organization_handlers.go:337`) from the canonical definitions in `types.Roles`, then never re-synced. If the canonical `RoleAdmin` config changes (e.g. `ResourceAny` was added later, or `Actions` were extended), existing orgs keep the old config. `the-linux-foundation` is an old production org, so the admin role's stored Config likely predates whatever change made `ResourceAny` work for projects.

Confirming this requires inspecting the actual `roles.config` row for that org. Steps included in the implementation plan.

## User stories

- **As an org member who has been added to a team that holds an `admin` access grant on a project**, I want to see that project in the org's project list and be able to open it, so I can do the work the team was given access to without needing org-owner escalation.
- **As an org member in a team with a `read` or `write` grant on a project**, I want the same baseline visibility — admin shouldn't be the only role that confers a "I can see this project exists" signal.
- **As an org owner**, my existing "see all projects" behaviour must not regress.

## Acceptance criteria

1. **Team-admin sees project (the reported regression).** Given an org member `U`, a team `T` containing `U`, and an `AccessGrant{TeamID=T, ResourceID=P, Roles=[admin]}` where `P.OrganizationID = T.OrganizationID`, the response of `GET /api/v1/projects?organization_id=<org>` to `U` includes project `P`. Verified end-to-end in the inner Helix.
2. **Team-read sees project.** Same setup with `Roles=[read]` — project visible.
3. **Team-write sees project.** Same setup with `Roles=[write]` — project visible.
4. **No-grant member is still excluded.** An org member with no team/user grant on `P` does NOT see `P`. (Negative case — confirms we did not turn the listing into "all org projects for everyone".)
5. **Org owner regression-test.** Org owners continue to see ALL projects in the org, including ones with no grants on them.
6. **Project owner regression-test.** A user who is the project's `UserID` continues to see the project even without team grants (they were always permitted; we must not accidentally remove the bypass).
7. **Existing orgs are repaired.** After the fix is deployed, existing orgs (including `the-linux-foundation`) have role rows in their `roles` table whose `config` JSONB matches the current canonical definitions in `types.Roles`. No DB migration script — done at server start.
8. **Test coverage.** `api/pkg/server/authz_test.go` gains a project-via-team table-driven test that covers AC 1–4. Style mirrors the existing `AuthzAppSuite` (mock store, `expectOrgMember`, `expectProjectAccess` etc.). Without these tests, this regression class would be invisible to CI again.

## Out of scope

- Letting non-admin team roles *write* to the project (`write` grants `Update`/`Delete` for `Application` and `Knowledge` only; whether they should also write `Project` is a product question — for this bug we only fix visibility, i.e. `ActionGet`).
- Changing the `RoleAdmin` `ResourceAny` semantics or rewriting the rule matcher.
- Refactoring the per-org role storage to be code-only (we keep the DB rows because custom roles are also stored there; we only ensure the canonical ones stay synced).
- Frontend changes — the bug is server-side, the frontend just renders whatever the API returns.
