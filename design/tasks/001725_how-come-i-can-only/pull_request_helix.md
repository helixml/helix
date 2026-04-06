# Fix GitHub repo browser to show public org repos and add org filter

## Summary
The GitHub repository browser only showed private repos from organizations. This was because the `GET /user/repos` API with `affiliation=organization_member` doesn't reliably return public org repos. Fixed by supplementing with per-org repo listing and deduplicating results. Also added an org filter dropdown to the browser UI for easier navigation.

## Changes
- **Backend** (`api/pkg/agent/skill/github/client.go`): `ListRepositories()` now also fetches the user's orgs via `GET /user/orgs` and lists each org's repos via `GET /orgs/{org}/repos`, then deduplicates by repo ID
- **Frontend** (`frontend/src/components/project/BrowseProvidersDialog.tsx`): Added an org/owner filter dropdown (derived from `full_name`) alongside the existing search field, with proper state reset on navigation
