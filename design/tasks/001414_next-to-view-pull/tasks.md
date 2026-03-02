# Implementation Tasks

## Backend

- [ ] Add `GET /api/v1/spec-tasks/{id}/branch-status` endpoint in `spec_task_workflow_handlers.go`
  - Reuse `services.GetDivergence()` to get commits ahead/behind
  - Return `{ commits_behind, commits_ahead, default_branch }`

- [ ] Add `POST /api/v1/spec-tasks/{id}/sync-branch` endpoint in `spec_task_workflow_handlers.go`
  - Validate task is in `pull_request` status
  - Send sync prompt to agent via `sendMessageToSpecTaskAgent()`

- [ ] Create `agent_sync_branch.tmpl` prompt template in `api/pkg/prompts/templates/`
  - Instructions for fetching and merging default branch
  - Conflict handling guidance
  - Push instructions

- [ ] Add `SyncBranchInstruction()` function in `api/pkg/prompts/helix_code_prompts.go`
  - Render template with branch names and commit count

- [ ] Add swagger annotations for new endpoints
- [ ] Run `./stack update_openapi` to generate client

## Frontend

- [ ] Add `useBranchStatus(taskId)` hook in `frontend/src/services/specTaskService.ts`
  - Calls `v1SpecTasksBranchStatusDetail` API
  - Polls every 30 seconds when task is in `pull_request` status
  - Returns `{ commitsBehind, commitsAhead, defaultBranch, isLoading }`

- [ ] Add `useSyncBranch()` mutation hook in `frontend/src/services/specTaskWorkflowService.ts`
  - Calls `v1SpecTasksSyncBranchCreate` API
  - Invalidates branch status query on success

- [ ] Update `SpecTaskActionButtons.tsx` for `pull_request` status section
  - Display branch status indicator next to "View Pull Request" button
  - Show "Sync Branch" button when `commitsBehind > 0`
  - Disable button when no agent session or sync in progress
  - Show loading spinner during sync

- [ ] Add status text/icons in `TaskCard.tsx` or inline in action buttons
  - "✓ Up to date" when not behind
  - "⚠ N behind main" when behind
  - "⏳ Syncing..." during operation

## Testing

- [ ] Test branch-status endpoint with various divergence states
- [ ] Test sync-branch endpoint triggers agent message
- [ ] Verify UI updates after successful sync
- [ ] Test disabled state when agent session disconnected