# Requirements: Enforce Agent Branch Restrictions Before Push Acceptance

## Problem Statement

Currently, when an agent pushes to an unauthorized branch (e.g., `main`), the Helix Git Server:
1. Accepts the push via `git receive-pack`
2. Detects the violation after the fact
3. Rolls back the refs server-side
4. Returns success to the client

This causes the agent to believe the push succeeded, creating a confusing "data loss" experience. The agent sees `main -> main` success message, but the commits are silently rejected.

## User Stories

### US1: Agent Receives Clear Rejection
As an agent pushing to an unauthorized branch, I want to receive a clear error message from git explaining why my push was rejected, so I understand what happened and can push to the correct branch.

### US2: No Silent Rollbacks  
As a user monitoring agent activity, I want push rejections to happen at the git protocol level (not via post-push rollback), so logs and client output accurately reflect what happened.

### US3: Branch Restrictions Enforced Consistently
As a project owner, I want agent branch restrictions to be enforced via git's pre-receive hook, so the protection is consistent with how GitHub/GitLab enforce branch protection rules.

## Acceptance Criteria

### AC1: Pre-Receive Hook Enforcement
- [ ] Branch restrictions are checked in the pre-receive hook BEFORE refs are updated
- [ ] Hook reads allowed branches from `HELIX_ALLOWED_BRANCHES` environment variable
- [ ] If env var is unset/empty, all branches are allowed (normal user behavior)

### AC2: Clear Error Messages
- [ ] Rejected pushes return GitHub-style error messages visible to the client
- [ ] Error message includes: which branch was rejected, which branches are allowed
- [ ] Example: `error: refusing to update 'main' - only these branches are allowed: helix-specs, feature/001234-my-task`

### AC3: No Rollback Code Path
- [ ] The post-receive rollback logic for branch restrictions is removed
- [ ] Refs are never updated for unauthorized branches (no need to rollback)

### AC4: Backward Compatibility
- [ ] Normal users (non-agent API keys) can push to any branch
- [ ] Existing force-push protection for `helix-specs` continues to work
- [ ] Hook handles both restrictions in a single script

## Out of Scope

- Web UI for configuring branch restrictions (uses existing spec task branch assignment)
- Per-user branch restrictions (only agent API keys are restricted)
- Wildcard branch patterns (exact branch name matching only)