# Design: Branch Sync Status & Auto-Merge Feature

## Architecture Overview

This feature adds branch divergence status to the TaskCard component and a mechanism to trigger the agent to merge the default branch into the feature branch.

## Components Modified

### Backend

1. **New API Endpoint**: `GET /api/v1/spec-tasks/{id}/branch-status`
   - Returns branch divergence info (commits ahead/behind of default branch)
   - Reuses existing `GetDivergence()` helper from `git_helpers.go`
   - Returns: `{ commits_behind: number, commits_ahead: number, default_branch: string }`

2. **New API Endpoint**: `POST /api/v1/spec-tasks/{id}/sync-branch`
   - Triggers agent to merge default branch into feature branch
   - Uses existing `sendMessageToSpecTaskAgent()` infrastructure
   - Returns task with updated status

3. **New Prompt Template**: `agent_sync_branch.tmpl`
   - Instructs agent to fetch and merge default branch
   - Includes conflict resolution instructions
   - Tells agent to push result or report conflicts

### Frontend

1. **TaskCard.tsx** - Add branch status indicator near "View Pull Request" button
2. **SpecTaskActionButtons.tsx** - Add "Sync Branch" button when behind
3. **New hook**: `useBranchStatus(taskId)` - Fetches branch divergence data

## Key Decisions

### Where to show status
Display inline with the "View Pull Request" button since that's the context where users care about branch divergence. Keep it compact.

### Polling vs real-time
Use polling (every 30s) for branch status. Real-time would require WebSocket infrastructure changes for minimal benefit.

### Error handling
If agent reports merge conflicts, show an alert on the TaskCard. User can then communicate with agent via existing chat.

## Existing Patterns Leveraged

- **Divergence calculation**: `services.GetDivergence()` already computes ahead/behind counts
- **Agent messaging**: `sendMessageToSpecTaskAgent()` sends prompts to connected agents
- **Prompt templates**: Existing `agent_rebase_required.tmpl` provides similar pattern
- **Branch status type**: `TypesExternalStatus` already has `commits_ahead/behind` fields

## Data Flow

```
User clicks "Sync Branch"
    → POST /api/v1/spec-tasks/{id}/sync-branch
    → Server sends prompt via sendMessageToSpecTaskAgent()
    → Agent receives instruction, runs git merge
    → Agent pushes or reports conflict
    → Frontend polls branch-status, sees updated state
```

## Prompt Template Design

```
Your branch "{{ .BranchName }}" is {{ .CommitsBehind }} commits behind "{{ .DefaultBranch }}".

Please merge the latest changes from {{ .DefaultBranch }}:

1. Fetch and merge:
   git fetch origin {{ .DefaultBranch }}
   git merge origin/{{ .DefaultBranch }}

2. If there are merge conflicts you cannot resolve automatically, stop and explain what files have conflicts and what help you need from the user.

3. If merge succeeds, push the updated branch:
   git push origin {{ .BranchName }}

4. Report the result to the user.
```

## UI Mockup

When branch is up to date:
```
[View Pull Request] ✓ Up to date
```

When branch is behind:
```
[View Pull Request] ⚠ 3 behind main [Sync Branch]
```

While syncing:
```
[View Pull Request] ⏳ Syncing... 
```

## Implementation Notes

### Gotchas
- Must fetch from remote before checking divergence (agent might have pushed)
- The task must have `planning_session_id` to send messages to agent
- Button should be disabled if no agent session is connected

### Testing
- Test with branch that's 0, 1, and many commits behind
- Test merge conflict scenario (agent should report, not crash)
- Test when agent session is disconnected (button disabled, tooltip explains)