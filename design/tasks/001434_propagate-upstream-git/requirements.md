# Requirements: Propagate Upstream Git Push Errors

## Problem Statement

When an agent pushes code to the Helix git proxy and the proxy fails to push that commit upstream (e.g., GitHub rejects with 403), the failure is silently swallowed. The agent and user see no indication that the push did not reach GitHub.

## User Stories

### US1: Operator Visibility
**As** a Helix operator  
**I want** to see upstream push failures in the repository status  
**So that** I can diagnose credential or permission issues

### US2: User Visibility  
**As** a user working with an agent  
**I want** to see push errors on my SpecTask  
**So that** I know when my branch hasn't reached GitHub

### US3: Error Recovery
**As** a user  
**I want** stale errors to clear on successful pushes  
**So that** I only see current failures

## Acceptance Criteria

1. **AC1**: When upstream push fails (e.g., 403 permission denied), `GitRepository.LastPushError` contains the error message and `Status` is set to `error`

2. **AC2**: When upstream push fails, `SpecTask.LastUpstreamPushError` contains a human-readable message for the affected task(s)

3. **AC3**: `GET /api/v1/git/repositories/{id}` returns `last_push_error`, `last_push_error_at`, and `status: error`

4. **AC4**: `GET /api/v1/spec-tasks/{id}` returns `last_upstream_push_error` and `last_upstream_push_error_at`

5. **AC5**: A subsequent successful push clears both error fields and resets `GitRepository.Status` to `active`

6. **AC6**: No changes to git wire protocol - agent's `git push` exit code is unaffected (architectural constraint)

## Out of Scope

- Real-time notifications to the agent (requires protocol changes)
- Automatic retry mechanisms
- UI changes (fields auto-surface via existing APIs)
