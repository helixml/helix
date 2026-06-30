# Implementation Tasks: Exclude Web-Hosting Sandboxes from the Sandboxes List

- [ ] Add `ExcludePurposes []string` field to `ListSandboxesQuery` in `api/pkg/store/store_sandboxes.go`
- [ ] Apply `query.Where("purpose NOT IN ?", q.ExcludePurposes)` when `ExcludePurposes` is non-empty in `ListSandboxes`
- [ ] In `Controller.List` (`api/pkg/sandbox/controller.go`), set `ExcludePurposes: []string{types.SandboxPurposeWebService}` by default
- [ ] (Optional, Story 3) Add `include_purposes`/`all_purposes` query param handling in `listOrgSandboxes` (`api/pkg/server/sandboxes_api_handlers.go`) and thread it to the controller
- [ ] (Optional) Add defence-in-depth client-side filter `s.purpose !== 'web-service'` in `frontend/src/pages/Sandboxes.tsx`
- [ ] Add a unit test for `ListSandboxes` with `ExcludePurposes`: web-service excluded, empty-purpose retained
- [ ] Verify other callers of `ListSandboxes` (provisioning, host-device queries) are unaffected (they pass no `ExcludePurposes`)
- [ ] Regression check: deploy a project web service, confirm it still runs and appears in the project Web Service UI, and is absent from the Sandboxes page
- [ ] Run backend tests and frontend build/lint
