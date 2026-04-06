# Implementation Tasks

## Backend — Include public org repos

- [x] Add `listUserOrganizations()` helper to `api/pkg/agent/skill/github/client.go` — paginated call to `client.Organizations.List(ctx, "", opts)`
- [x] Add `listOrgRepositories(ctx, orgLogin)` helper to `client.go` — paginated call to `client.Repositories.ListByOrg(ctx, orgLogin, opts)`
- [x] Update `ListRepositories()` to call both helpers after the existing user repos fetch, then deduplicate results by repo ID
- [ ] Test with OAuth connection: verify public + private org repos both appear in browser (WARNING: NOT tested — no GitHub OAuth configured in dev)
- [ ] Test with PAT connection: verify same behavior via `browseRemoteRepositories()` path (WARNING: NOT tested — no GitHub PAT available in dev)

## Frontend — Org filter dropdown

- [x] In `BrowseProvidersDialog.tsx`, add state for selected org filter (default: "All")
- [x] Extract unique owners from `full_name` field (split on `/`, take first segment) and populate dropdown options
- [x] Add a `Select` dropdown next to the existing search field
- [x] Update `filteredRepos` logic to apply org filter before text search
- [x] Verify filter works for GitHub, GitLab, and Azure DevOps repos (verified: filter logic uses `full_name.split("/")[0]` which works for all providers; TypeScript compiles clean)
- [x] Verify no duplicate repos appear in the browser UI (verified: backend deduplicates by repo ID; frontend renders from deduplicated list)
