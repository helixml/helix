# Design: Show Public Org Repos in GitHub Repository Browser

## Architecture

The fix is in `api/pkg/agent/skill/github/client.go` — the `ListRepositories()` function. Two callers use it:

1. **OAuth path**: `api/pkg/server/oauth.go:1082` — `handleListOAuthConnectionRepositories()`
2. **PAT path**: `api/pkg/server/git_repository_handlers.go:1722` — `browseRemoteRepositories()`

Both call `ghClient.ListRepositories(ctx)` and convert the result to `[]types.RepositoryInfo`. The fix is entirely in the shared `ListRepositories()` method — no caller changes needed.

## The Problem

GitHub's `GET /user/repos` API with `affiliation=organization_member` returns repos where the user has access **through org membership**. Public repos in an org are accessible to everyone, not specifically through membership, so GitHub may exclude them from this affiliation filter.

## Solution: Supplement with Org-Level Repo Listing

Extend `ListRepositories()` to also fetch repos from the user's organizations:

1. **Keep existing call**: `GET /user/repos` with current params (gets personal, collaborator, and some org repos)
2. **Add org listing**: Call `GET /user/orgs` to get the user's organizations
3. **Add per-org repo listing**: For each org, call `GET /orgs/{org}/repos` which returns **all** repos in that org visible to the authenticated user (public + private)
4. **Deduplicate**: Merge results by repo ID to avoid duplicates

### Pseudocode

```go
func (c *Client) ListRepositories(ctx context.Context) ([]*github.Repository, error) {
    // Step 1: Existing user repos call (unchanged)
    allRepos := listUserRepos(ctx) // owner + collaborator + organization_member

    // Step 2: List user's organizations
    orgs := c.client.Organizations.List(ctx, "", opts)

    // Step 3: For each org, list all visible repos
    for _, org := range orgs {
        orgRepos := c.client.Repositories.ListByOrg(ctx, org.GetLogin(), opts)
        allRepos = append(allRepos, orgRepos...)
    }

    // Step 4: Deduplicate by repo ID
    return deduplicate(allRepos), nil
}
```

### Why This Approach

- **`ListByOrg`** (`GET /orgs/{org}/repos`) returns all repos visible to the authenticated user, including public ones — this is the key missing piece
- **Deduplication** is necessary because some repos will appear in both the user repos list and org repos list
- **No scope changes needed**: The existing OAuth token with `repo` scope already has permission to list org repos

### Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where to fix | `client.go` only | Single shared method, both callers benefit |
| How to get public org repos | `ListByOrg` per org | GitHub API design requires this; no single endpoint returns everything |
| Dedup strategy | By repo ID | Stable, unique identifier from GitHub |
| Pagination | Same pattern as existing | Use `PerPage: 100` + `NextPage` loop for orgs and org repos |

### Performance Consideration

Users in many orgs will see additional API calls (1 for org list + N for each org). This is acceptable because:
- GitHub API rate limit is 5000/hr for authenticated users
- Most users are in <10 orgs
- This is a user-initiated browse action, not a background poll

## Frontend: Org Filter Dropdown

### Current State

`BrowseProvidersDialog.tsx` already has:
- A text search input filtering by `name`, `full_name`, and `description` (line 620-628)
- A flat list of repos with `full_name` as primary text (e.g. `gatewaze/repo-name`)
- No org/owner grouping or filtering

### Enhancement

Add an org/owner filter dropdown alongside the existing search field. This is a **client-side only** change — no new API endpoints needed.

1. **Extract unique owners** from the repo list by splitting `full_name` on `/` and taking the first segment
2. **Render a dropdown** (MUI `Select` or `Autocomplete`) with options: `["All", "gatewaze", "personal-user", ...]`
3. **Apply filter** before the existing text search filter — chain both filters together

### Why client-side only

The `full_name` field already contains the owner prefix for all providers (GitHub: `org/repo`, GitLab: `group/project`, Azure DevOps: `project/repo`). No backend changes or new fields are needed — we just parse what's already there.
