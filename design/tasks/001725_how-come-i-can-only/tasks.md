# Implementation Tasks

## Backend — Include public org repos

- [x] Add `listUserOrganizations()` helper to `api/pkg/agent/skill/github/client.go` — paginated call to `client.Organizations.List(ctx, "", opts)`
- [x] Add `listOrgRepositories(ctx, orgLogin)` helper to `client.go` — paginated call to `client.Repositories.ListByOrg(ctx, orgLogin, opts)`
- [x] Update `ListRepositories()` to call both helpers after the existing user repos fetch, then deduplicate results by repo ID
- [ ] Test with OAuth connection: verify public + private org repos both appear in browser
- [ ] Test with PAT connection: verify same behavior via `browseRemoteRepositories()` path

## Frontend — Org filter dropdown

- [x] In `BrowseProvidersDialog.tsx`, add state for selected org filter (default: "All")
- [x] Extract unique owners from `full_name` field (split on `/`, take first segment) and populate dropdown options
- [x] Add a `Select` dropdown next to the existing search field
- [x] Update `filteredRepos` logic to apply org filter before text search
- [ ] Verify filter works for GitHub, GitLab, and Azure DevOps repos
- [ ] Verify no duplicate repos appear in the browser UI
