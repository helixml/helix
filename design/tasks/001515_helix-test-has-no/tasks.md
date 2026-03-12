# Implementation Tasks

- [ ] Add `--organization` / `-o` string flag to `NewTestCmd()` in `api/cmd/helix/test.go`
- [ ] Add `organization` parameter to `runTest()` signature and pass it from `NewTestCmd`'s `RunE`
- [ ] In `deployApp()`, add `organization` parameter; when non-empty, call `cli.LookupOrganization()` and set `app.OrganizationID` on the created app (same pattern as `helix apply`'s `createApp` in `api/pkg/cli/app/apply.go`)
- [ ] In `deleteApp()`, add `organizationID` parameter; pass it into `client.AppFilter{OrganizationID: orgID}` so `ListApps` is scoped correctly for cleanup
- [ ] Thread the organization string from `runTest()` through to both `deployApp()` and `deleteApp()` call sites
- [ ] Add `"github.com/helixml/helix/api/pkg/cli"` import to `api/cmd/helix/test.go` (for `cli.LookupOrganization`)
- [ ] Build: `cd api && go build ./cmd/helix/`
- [ ] Manual test: `helix test -f helix.yaml -o <org-name>` — verify app creates in org, tests run, app is cleaned up
- [ ] Manual test: `helix test -f helix.yaml` (no org flag) — verify backward compatibility still works