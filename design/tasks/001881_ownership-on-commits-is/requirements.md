# Requirements: Fix Commit Ownership to Match Approving User

## Problem

When User 1 creates a task and User 2 approves the specs (transitioning to implementation), the AI agent's commits are authored as User 1 (the task creator) instead of User 2 (the approver). The PR is correctly created as User 2 because PR creation already uses the approver's OAuth, but commits use the creator's identity because `GIT_USER_NAME`/`GIT_USER_EMAIL` are set at container startup from `task.CreatedBy`.

See: https://github.com/helixml/helix/pull/2250 — PR opened by `chocobar`, but all commits authored by `lukemarsden` (the task creator).

## User Stories

1. **As a reviewer who approves specs**, I want commits made during implementation to be attributed to me, since I took ownership by transitioning the task to implementation.

2. **As a reviewer without GitHub OAuth connected**, I want to be prompted to connect GitHub OAuth before the task transitions to implementation, the same way I'm prompted before PR creation.

## Acceptance Criteria

- [ ] When a user approves specs (transitions task to implementation), commits made by the agent are authored with the **approving user's** name and email, not the task creator's.
- [ ] If the approving user doesn't have GitHub OAuth connected and the repo is GitHub-hosted, the system returns an `oauth_required` error (same pattern as `approveImplementation`).
- [ ] The PR creation continues to use the approving user's credentials (no regression).
- [ ] The git push credentials use the approving user's OAuth token (not the task creator's).
- [ ] Enterprise ADO deployments still enforce corporate email validation (no regression).
