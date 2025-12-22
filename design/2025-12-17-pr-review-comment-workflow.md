# PR Review Comment Workflow

**Status:** Roadmap / Not Yet Implemented
**Date:** 2025-12-17

## Overview

Enable automated handling of PR review comments from Azure DevOps. When reviewers leave comments on a PR, the system should automatically route those comments to the agent for resolution, creating a feedback loop until the PR is approved.

## User Story

As a developer using Helix with Azure DevOps, I want review comments on my PR to be automatically sent to the agent so that it can address feedback without manual intervention.

## Proposed Workflow

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Pull Request   │────▶│  Implementation │────▶│  Pull Request   │
│    (waiting)    │     │  (addressing)   │     │   (updated)     │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        ▲                       │                       │
        │                       │                       │
        └───────────────────────┴───────────────────────┘
              New comments trigger re-work cycle
```

1. Task is in `pull_request` status, awaiting merge
2. Reviewer leaves comment on ADO PR
3. System detects new unaddressed comment via polling
4. Task moves back to `implementation` status
5. Agent receives the review comment with instructions to address it
6. Agent makes changes, commits, and pushes to feature branch
7. System detects new commit, moves task back to `pull_request` status
8. Cycle repeats until PR is merged or abandoned

## Key Design Decisions

### 1. Determining if a comment is "addressed"

**Recommended: Option B - Commit-based detection**

Check if there's a new commit on the feature branch after the comment timestamp. This is simple and reliable.

Alternatives considered:
- Option A: Track comment IDs sent to agent (requires state management)
- Option C: User manually marks as resolved (adds friction)

### 2. Review feedback source

**Recommended: Option A - ADO comments only (initially)**

Poll ADO for PR comments and forward to agent. Keep it simple.

Future enhancement: Allow users to submit feedback directly in Helix UI, which could optionally create an ADO PR comment.

### 3. Detecting agent push completion

**Recommended: Option A - Poll for new commits**

Poll the git repository for new commits on the feature branch. Consistent with existing polling patterns.

Alternative: Agent calls API to signal completion (requires agent-side changes).

## Technical Implementation

### Backend Changes

1. **ADO API Integration** (`api/pkg/git/ado_client.go`)
   - Add `GetPullRequestComments(prId string) ([]PRComment, error)`
   - Add `GetBranchCommits(branch string, since time.Time) ([]Commit, error)`

2. **New SpecTask Fields** (`api/pkg/types/simple_spec_task.go`)
   ```go
   LastCommentCheckAt    *time.Time `json:"last_comment_check_at,omitempty"`
   LastAddressedCommitAt *time.Time `json:"last_addressed_commit_at,omitempty"`
   PendingReviewComments []string   `json:"pending_review_comments,omitempty" gorm:"-"`
   ```

3. **PR Comment Polling** (`api/pkg/services/spec_task_orchestrator.go`)
   - In `handlePullRequest`, also check for new comments
   - If unaddressed comments found:
     - Move task to `implementation` status
     - Send review comment to agent via instruction service

4. **Agent Instruction** (`api/pkg/services/agent_instruction_service.go`)
   - Add `SendReviewCommentInstruction(task, comment)`
   - Prompt template:
     ```
     A reviewer has left the following comment on your pull request:

     ---
     {comment}
     ---

     Please address this feedback. When you've made the necessary changes:
     1. Commit your changes with a descriptive message
     2. Push to the feature branch

     The PR will be automatically updated.
     ```

5. **Commit Detection** (in `prPollLoop`)
   - After moving task back to `pull_request`, check for new commits
   - If new commit detected after `LastAddressedCommitAt`, task stays in PR status

### Frontend Changes

1. **TaskCard Enhancement**
   - Show pending review comments count badge
   - Display latest review comment preview

2. **PR Column Enhancement**
   - Show "Addressing feedback" indicator when task cycles back to implementation
   - Show review comment history

3. **Future: Direct Review Feedback**
   - Add text input in Helix UI for submitting review feedback
   - Option to also post as ADO PR comment

## Data Flow

```
ADO PR Comment
      │
      ▼
┌─────────────────────┐
│  prPollLoop (1min)  │
│  - Check PR status  │
│  - Check comments   │
└─────────────────────┘
      │
      ▼ (new comment detected)
┌─────────────────────┐
│  Move to impl       │
│  Send to agent      │
└─────────────────────┘
      │
      ▼ (agent pushes)
┌─────────────────────┐
│  Detect new commit  │
│  Move to PR status  │
└─────────────────────┘
```

## API Changes

No new endpoints required initially. Existing polling handles state transitions.

Future: `POST /api/v1/spec-tasks/{id}/review-feedback` for Helix UI feedback submission.

## Testing Plan

1. Unit tests for ADO comment fetching
2. Unit tests for comment-addressed detection logic
3. Integration test for full review cycle
4. Manual testing with real ADO PR

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| ADO rate limiting | Respect 1-minute polling interval, batch requests |
| Agent doesn't address comment correctly | User can manually intervene via ADO |
| Infinite review loop | Add max retry count, alert after N cycles |
| Comment detection false positives | Only consider comments from non-agent users |

## Future Enhancements

1. Support GitHub PR reviews (when GitHub integration is added)
2. Inline code suggestions from reviews
3. Review comment threading/conversation support
4. Automatic PR approval request after addressing comments

## Dependencies

- Existing ADO integration for PR status polling
- Agent instruction service
- prPollLoop infrastructure

## Effort Estimate

- Backend: ~2-3 days
- Frontend: ~1-2 days
- Testing: ~1 day
- Total: ~4-6 days

## References

- Azure DevOps REST API: Pull Request Comments
- Existing PR polling: `api/pkg/services/spec_task_orchestrator.go`
