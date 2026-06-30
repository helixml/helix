# Requirements: Exclude Web-Hosting Sandboxes from the Sandboxes List

## Background / Investigation Result

**Question asked:** Do web-hosting sandboxes show up in the list of sandboxes in the
Sandboxes UI & API? If not, why not?

**Answer: Yes, they currently DO show up.** There is no filtering — by purpose or
otherwise — anywhere in the list path. The org-scoped list endpoint returns every
sandbox for the org, and the frontend renders the list unmodified.

What makes a web-hosting sandbox different at the data level:

| Field            | Ordinary sandbox      | Web-hosting sandbox        |
|------------------|-----------------------|----------------------------|
| `Purpose`        | `""` (empty)          | `"web-service"`            |
| `Persistent`     | usually `false`       | always `true`              |
| `TimeoutSeconds` | positive (e.g. 3600)  | `-1` (never auto-expire)   |
| `ProjectID`      | optional              | always set                 |

Web-hosting sandboxes are **not created by users**. They are created and managed
automatically by the project Web Service feature (`api/pkg/webservice/controller.go`)
when a project's web service is deployed. They are surfaced through the project's
**Web Service** UI, not the generic Sandboxes list.

**Why this is a problem:** Because they appear in the generic Sandboxes list with no
distinction, a user can manually delete or interfere with a sandbox that is actively
hosting a project's web service, and the list is cluttered with infrastructure the
user did not create and is not meant to manage there.

Relevant files (verified):
- `api/pkg/server/sandboxes_api_handlers.go` — `listOrgSandboxes` handler
- `api/pkg/sandbox/controller.go` — `Controller.List(ctx, orgID, projectID)`
- `api/pkg/store/store_sandboxes.go` — `ListSandboxesQuery` / `ListSandboxes`
- `api/pkg/types/sandbox.go` — `Sandbox.Purpose`, `SandboxPurposeWebService = "web-service"`
- `frontend/src/pages/Sandboxes.tsx`, `frontend/src/services/sandboxesService.ts`

## User Stories

### Story 1 — Clean Sandboxes list
**As a** Helix user viewing the Sandboxes page,
**I want** the list to show only the sandboxes I created/manage,
**so that** internal web-hosting sandboxes don't clutter the view or get deleted by mistake.

**Acceptance Criteria:**
- [ ] The Sandboxes UI no longer lists sandboxes with `Purpose == "web-service"`.
- [ ] The list API (`GET /api/v1/organizations/{org_id}/sandboxes`) excludes
      `web-service` sandboxes by default.
- [ ] Ordinary sandboxes (empty `Purpose`) continue to appear exactly as before.

### Story 2 — Web service still functions
**As a** project owner using the Web Service feature,
**I want** the web-hosting sandbox to keep running and remain visible in the
project Web Service UI,
**so that** hiding it from the generic list does not break web hosting.

**Acceptance Criteria:**
- [ ] Web service deploy / redeploy / status behaviour is unchanged.
- [ ] The web-hosting sandbox is still tracked via `ProjectWebServiceState.ActiveSandboxID`
      and reachable through the project Web Service endpoints.
- [ ] Filtering is applied **only** to the user-facing list path; internal callers of
      `ListSandboxes` (provisioning, host-device queries, etc.) are unaffected.

### Story 3 — Optional explicit inclusion (admin/debug)
**As an** operator debugging the system,
**I want** an explicit way to include web-service sandboxes in the list response,
**so that** they remain discoverable when intentionally requested.

**Acceptance Criteria:**
- [ ] A query parameter (e.g. `?include_purposes=web-service` or `?all_purposes=true`)
      returns web-service sandboxes when explicitly set. (Optional — see design.)

## Out of Scope
- Redesigning the Web Service UI.
- Changing how web-hosting sandboxes are created or provisioned.
- Adding new sandbox purposes.
