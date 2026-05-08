# Fix GitHub repo picker: require read:org scope for org repo listing

## Summary

Task 1725 added a 4-step `ListRepositories()` to fetch public org repos, but Step 2 (`GET /user/orgs`) silently fails without `read:org` scope — returning 403 per GitHub API docs. This meant Steps 2-3 never ran, so public org repos were still missing.

Additionally, GitHub OAuth App org-level access restrictions can silently prevent org data from being returned even with correct scopes. The UI now explains this and links to the settings page.

## Changes

- Add `read:org` to `GITHUB_REQUIRED_SCOPES` — existing OAuth connections without it now trigger scope upgrade banner
- Update scope upgrade warning to list all required scopes (`repo`, `workflow`, `read:org`)
- Update PAT helper text to mention `read:org` scope requirement
- Add server-side warning log when PAT is missing `read:org`
- Add warning log when `listUserOrganizations()` fails (was silent fallback)
- Sort repo list alphabetically by full_name in browse view
- Add info Alert explaining GitHub OAuth App org access restrictions with link to `https://github.com/settings/connections/applications/{client_id}`

## Root Cause

Two issues:

1. **Missing scope**: GitHub's `GET /user/orgs` endpoint requires `read:org` or `user` scope. Without it, the call returns 403, which was caught and silently swallowed. The OAuth auth URL already requests `read:org`, but existing connections created before that change didn't have it, and `GITHUB_REQUIRED_SCOPES` only checked for `["repo", "workflow"]`.

2. **Org access restrictions**: GitHub allows organizations to restrict which OAuth apps can access their data. Even with correct scopes, restricted orgs are invisible to the API. Users need to grant access at the GitHub settings page. The UI now explains this clearly.
