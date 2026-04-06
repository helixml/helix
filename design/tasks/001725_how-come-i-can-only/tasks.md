# Implementation Tasks

- [ ] Add `listUserOrganizations()` helper to `api/pkg/agent/skill/github/client.go` — paginated call to `client.Organizations.List(ctx, "", opts)`
- [ ] Add `listOrgRepositories(ctx, orgLogin)` helper to `client.go` — paginated call to `client.Repositories.ListByOrg(ctx, orgLogin, opts)`
- [ ] Update `ListRepositories()` to call both helpers after the existing user repos fetch, then deduplicate results by repo ID
- [ ] Test with OAuth connection: verify public + private org repos both appear in browser
- [ ] Test with PAT connection: verify same behavior via `browseRemoteRepositories()` path
- [ ] Verify no duplicate repos appear in the browser UI
