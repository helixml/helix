# Fix GitHub repo picker: require read:org scope for org repo listing

## Summary

Task 1725 added a 4-step `ListRepositories()` to fetch public org repos, but Step 2 (`GET /user/orgs`) silently fails without `read:org` scope — returning 403 per GitHub API docs. This meant Steps 2-3 never ran, so public org repos were still missing.

## Changes

- Add `read:org` to `GITHUB_REQUIRED_SCOPES` — existing OAuth connections without it now trigger scope upgrade banner
- Update scope upgrade warning to list all required scopes (`repo`, `workflow`, `read:org`)
- Update PAT helper text to mention `read:org` scope requirement
- Add server-side warning log when PAT is missing `read:org`
- Add warning log when `listUserOrganizations()` fails (was silent fallback)

## Root Cause

GitHub's `GET /user/orgs` endpoint requires `read:org` or `user` scope. Without it, the call returns 403, which was caught and silently swallowed at `client.go:136`. The OAuth auth URL already requests `read:org`, but existing connections created before that change didn't have it, and `GITHUB_REQUIRED_SCOPES` only checked for `["repo", "workflow"]`.
