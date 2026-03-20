# Design: Multi-Repository PR Support

## Overview

Extend the SpecTask model and UI to track and display pull requests across all repositories attached to a project, not just the primary repository.

## Current Architecture

```
SpecTask
├── BranchName        (string)     # Single branch name
├── PullRequestID     (string)     # Single PR ID  
└── PullRequestURL    (string)     # Single PR URL (computed)
```

The `ensurePullRequestForTask()` function in `spec_task_workflow_handlers.go` only operates on the primary repository.

## Proposed Architecture

### New Data Model

```
SpecTask
├── BranchName        (string)           # Branch name (same across all repos)
├── PullRequestID     (string)           # DEPRECATED: Keep for backward compat
├── PullRequestURL    (string)           # DEPRECATED: Keep for backward compat
└── RepoPullRequests  ([]RepoPR, JSON)   # NEW: Per-repo PR tracking
```

```go
type RepoPR struct {
    RepositoryID   string `json:"repository_id"`
    RepositoryName string `json:"repository_name"`
    PRID           string `json:"pr_id"`
    PRNumber       int    `json:"pr_number"`
    PRURL          string `json:"pr_url"`
    PRState        string `json:"pr_state"` // "open", "closed", "merged"
}
```

### Key Changes

1. **Backend: `spec_task_workflow_handlers.go`**
   - `ensurePullRequestForTask()` → iterate over all project repos with external URLs
   - Track PR info per repo in new `RepoPullRequests` field

2. **Backend: `git_http_server.go`**
   - `handleFeatureBranchPush()` → detect pushes to any repo, not just primary
   - Trigger PR creation for the specific repo that received the push

3. **API: Return full PR list**
   - `GetSpecTask` response includes `repo_pull_requests` array
   - Computed `pull_request_url` still populated from primary repo for backward compat

4. **Frontend: `SpecTaskActionButtons.tsx`**
   - "View Pull Request" button becomes dropdown if multiple PRs
   - Show repo name prefix for each PR link

## Decision Log

| Decision | Rationale |
|----------|-----------|
| Store PRs as JSON array on SpecTask | Simpler than separate join table; PR count per task is small (<10) |
| Keep deprecated single-PR fields | Backward compatibility with existing API consumers |
| Same branch name across all repos | Simplifies tracking; mirrors how multi-repo projects typically work |

## Migration

No database migration required. New `RepoPullRequests` field is JSON column with default empty array. Existing tasks continue to work via deprecated fields.

## Implementation Notes

### Files Modified

**Backend:**
- `api/pkg/types/simple_spec_task.go` - Added `RepoPR` struct and `RepoPullRequests` field to `SpecTask`
- `api/pkg/server/spec_task_workflow_handlers.go` - Refactored `ensurePullRequestForTask()` to `ensurePullRequestForRepo()`, added `ensurePullRequestsForAllRepos()` and `updateRepoPullRequests()` helper
- `api/pkg/services/git_http_server.go` - Updated `ensurePullRequest()` to populate `RepoPullRequests` array via new `updateRepoPullRequests()` helper
- `api/pkg/server/spec_driven_task_handlers.go` - Updated `getTask()` and `listTasks()` to compute PR URLs for `RepoPullRequests` array

**Frontend:**
- `frontend/src/components/tasks/SpecTaskActionButtons.tsx` - Added `RepoPR` interface, `repo_pull_requests` field to `SpecTaskForActions`, and dropdown menu for multiple PRs

### Key Patterns

- PR URLs are computed on-the-fly in API handlers using `services.GetPullRequestURL()` rather than stored
- `listTasks()` batches repo lookups to avoid N+1 queries when computing PR URLs
- Frontend uses MUI `Menu` component for PR dropdown; only shows dropdown when >1 PR exists
- Single PR case maintains original UX (direct button click opens PR)

### Backward Compatibility

- Deprecated `PullRequestID` and `PullRequestURL` fields still populated from primary repo
- Existing API consumers continue to work without changes
- Frontend falls back to `pull_request_url` if `repo_pull_requests` is empty