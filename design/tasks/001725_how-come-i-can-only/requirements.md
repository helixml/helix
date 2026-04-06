# Requirements: Show Public Org Repos in GitHub Repository Browser

## Problem

When browsing GitHub repositories via the Helix repo browser (e.g. at `/orgs/helix?tab=repositories`), only **private** repos from an organization are shown. Public repos the user has access to are missing.

**Example:** User is a member of `gatewaze` org with write access to all 5 repos. The browser only shows the 2 private repos (`premium-gatewaze-modules`, `lf-gatewaze-agents`) and hides the 3 public ones (`gatewaze`, `gatewaze-modules`, `lf-gatewaze-modules`).

## Root Cause

`ListRepositories()` in `api/pkg/agent/skill/github/client.go` calls the GitHub REST API `GET /user/repos` with `Affiliation: "owner,collaborator,organization_member"`. For org public repos, GitHub's API may not include them under `organization_member` affiliation since access isn't membership-gated — they're publicly accessible. The client has no org-specific listing code to supplement these results.

## User Stories

1. **As a user**, I want to see all repos (public and private) from my GitHub organizations in the Helix repo browser, so I can use Helix to develop on any repo I have access to.

2. **As a user**, I want public repos I collaborate on (outside my orgs) to also appear in the browser.

## Acceptance Criteria

- [ ] The GitHub repo browser shows **all** repos the authenticated user can access, including public org repos
- [ ] Both OAuth and PAT code paths return the same comprehensive repo list
- [ ] Results are deduplicated (a repo appears only once even if accessible via multiple affiliations)
- [ ] No performance regression for users with many orgs/repos (pagination still works)
- [ ] Existing private repo visibility is not affected
