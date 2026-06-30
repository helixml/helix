# Design: Exclude Web-Hosting Sandboxes from the Sandboxes List

## Summary

Today the Sandboxes list shows every sandbox in the org, including the internal
`web-service` sandboxes created by the project Web Service feature. The fix is to
filter out `Purpose == "web-service"` sandboxes from the **user-facing list path
only**, leaving creation, provisioning, and all other internal queries untouched.

## Current Behaviour (verified)

1. UI: `frontend/src/pages/Sandboxes.tsx` calls `useListSandboxes(orgId)`
   → `apiClient.v1OrganizationsSandboxesDetail(orgId)` and renders the array as-is.
2. API: `listOrgSandboxes` (`api/pkg/server/sandboxes_api_handlers.go`) →
   `sandboxController.List(ctx, orgID, projectID)`.
3. Controller: `Controller.List` (`api/pkg/sandbox/controller.go`) →
   `store.ListSandboxes(&ListSandboxesQuery{OrganizationID, ProjectID})`.
4. Store: `ListSandboxes` (`api/pkg/store/store_sandboxes.go`) applies WHERE clauses
   for OrganizationID, ProjectID, Owner, Status, HostDeviceID, IncludeDeleted —
   **no purpose filter**.

`ListSandboxes` is shared by other callers (provisioning, host-device reconciliation),
so it must NOT change behaviour globally.

## Approach

Add an **opt-in exclusion** to the query struct and apply it only on the user-facing
list path. Default the user-facing path to excluding `web-service`.

### Backend

1. **Store** — `api/pkg/store/store_sandboxes.go`
   - Add a field to `ListSandboxesQuery`:
     ```go
     ExcludePurposes []string // exclude rows whose purpose is in this set
     ```
   - In `ListSandboxes`, add:
     ```go
     if len(q.ExcludePurposes) > 0 {
         query = query.Where("purpose NOT IN ?", q.ExcludePurposes)
     }
     ```
   - Note: `purpose` is `""` for ordinary sandboxes, which is correctly retained
     by `NOT IN ("web-service")`.

2. **Controller** — `api/pkg/sandbox/controller.go`
   - In `Controller.List`, set `ExcludePurposes: []string{types.SandboxPurposeWebService}`
     by default. To support Story 3, accept an optional flag/param so the handler can
     request inclusion.

3. **Handler** — `api/pkg/server/sandboxes_api_handlers.go`
   - (Optional, Story 3) Read `include_purposes` / `all_purposes` query param and pass
     through to the controller. If skipping Story 3, the controller always excludes
     `web-service`.

### Frontend

No change strictly required once the API excludes them — the list simply won't contain
web-service rows. As defence-in-depth and to keep the change self-contained, optionally
also filter client-side in `frontend/src/pages/Sandboxes.tsx`:
```ts
const sandboxes = (data?.sandboxes ?? []).filter(s => s.purpose !== 'web-service')
```
Prefer the backend filter as the source of truth; treat the client filter as optional.

## Key Decisions

- **Filter, don't delete-protect.** Hiding from the generic list is simpler and matches
  the mental model: these are managed by the Web Service feature, surfaced there.
  Delete-protection in the generic UI is unnecessary once they're not listed.
- **Opt-in exclusion field, not a hard-coded WHERE in `ListSandboxes`.** Keeps the shared
  store function neutral; only the user-facing controller path opts into exclusion.
- **Exclude by default rather than show-with-badge.** Users did not create these and have
  no actions to take on them in the generic list; showing them adds risk and noise. (A
  badge-and-show alternative was considered but rejected as more UI work for less safety.)
- **`NOT IN` with a slice** keeps it extensible if future internal purposes are added.

## Testing

- Unit test `ListSandboxes` with `ExcludePurposes` set: web-service row excluded,
  empty-purpose rows retained.
- Verify the org list endpoint omits a web-service sandbox while still returning ordinary
  ones for the same org/project.
- Manual/regression: deploy a project web service, confirm it still runs and appears in
  the project Web Service UI, and confirm it is absent from the Sandboxes page.

## Risks

- If any existing code relied on the generic list including web-service sandboxes, it
  would now miss them. Verified: only the user-facing UI consumes this endpoint; the
  Web Service feature uses `ActiveSandboxID` / project endpoints, not this list.
