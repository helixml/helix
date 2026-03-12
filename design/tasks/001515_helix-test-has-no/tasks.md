# Implementation Tasks

- [~] Add `ResolveOrganization()` helper to `api/pkg/cli/organization.go` — takes org flag string, returns resolved org ID; if flag is empty, calls `ListOrganizations()` and returns the first org's ID; if no orgs exist, returns empty string
- [ ] Add `--organization` / `-o` string flag to `NewTestCmd()` in `api/cmd/helix/test.go`
- [ ] Add `organization` parameter to `runTest()` signature and call `cli.ResolveOrganization()` early to resolve it to an org ID
- [ ] Add `organizationID` parameter to `deployApp()`; set `app.OrganizationID` when non-empty
- [ ] Add `organizationID` parameter to `deleteApp()`; pass it into `client.AppFilter{OrganizationID: orgID}` so `ListApps` is scoped correctly for cleanup
- [ ] Add `"github.com/helixml/helix/api/pkg/cli"` import to `api/cmd/helix/test.go` (for `cli.ResolveOrganization`)
- [ ] Update `helix apply` in `api/pkg/cli/app/apply.go` — replace the existing `LookupOrganization` block in `createApp()` with a call to `cli.ResolveOrganization()` so it also defaults to the user's first org
- [ ] Build: `cd api && go build ./cmd/helix/`
- [ ] Manual test: `helix test -f helix.yaml -o <org-name>` — verify app creates in org, tests run, app is cleaned up
- [ ] Manual test: `helix test -f helix.yaml` (no org flag) — verify it defaults to first org
- [ ] Manual test: `helix apply -f helix.yaml` (no org flag) — verify it defaults to first org