# Requirements: Fix public org repos still missing in GitHub repo picker

## Context

Task 1725 added a 4-step process to `ListRepositories()` in `api/pkg/agent/skill/github/client.go`: (1) fetch user repos, (2) list user's orgs, (3) fetch repos per org, (4) deduplicate. An org filter dropdown was also added to the frontend `BrowseProvidersDialog.tsx`. Despite merging and deploying this, public org repos still don't appear when filtering by org.

## Root Cause

GitHub's `GET /user/orgs` endpoint requires `read:org` (or `user`) scope on the token. Without it, the endpoint returns 403 or an empty list. The current code silently swallows this failure (line 136 of `client.go` returns Step 1 results as non-fatal), so Steps 2-3 never run and public org repos from per-org listing are lost.

Three contributing factors:

1. **OAuth scope upgrade check is incomplete** — `GITHUB_REQUIRED_SCOPES` in `BrowseProvidersDialog.tsx:74` is `["repo", "workflow"]`. Existing OAuth connections authorized before `read:org` was added to the auth URL (line 423) don't trigger a scope upgrade, so they proceed with a token that can't list orgs.

2. **PAT validation ignores `read:org`** — `browseRemoteRepositories()` in `git_repository_handlers.go:1710-1719` only validates the `repo` scope. Users creating a PAT with only `repo` scope get no warning that org listing won't work.

3. **Silent fallback hides the problem** — `listUserOrganizations()` errors are swallowed at `client.go:136`. The user sees repos from Step 1 only, with no indication that org-level listing was skipped.

## User Stories

1. **As a user with an existing OAuth connection**, I want the system to detect that my token is missing `read:org` scope and prompt me to re-authorize, so that I can see public org repos.

2. **As a user creating a PAT**, I want clear guidance that `read:org` scope is needed (in addition to `repo`), so that the org listing works on first try.

3. **As a user browsing repos**, I want to know if org-level listing was skipped (e.g. due to permissions), so I understand why some repos might be missing.

## Acceptance Criteria

- [ ] `GITHUB_REQUIRED_SCOPES` includes `read:org` — existing OAuth connections without it trigger scope upgrade prompt
- [ ] PAT helper text mentions `read:org` scope requirement alongside `repo`
- [ ] PAT scope validation warns (non-blocking) when `read:org` is absent
- [ ] Backend logs a warning when `listUserOrganizations()` fails (instead of silent fallback)
- [ ] After re-authorizing OAuth with `read:org`, public org repos appear in the picker
- [ ] Org filter dropdown correctly filters to show only repos from the selected org
- [ ] Works for both OAuth and PAT code paths
