# Design: Move Back to Backlog Button

## Overview

Add a "Move to Backlog" action for tasks that are currently in progress (planning, review, or implementation phases). This allows users to defer work on a task without archiving or deleting it.

## Architecture

### Existing Patterns Used

- **Status updates via `useUpdateSpecTask`**: Already used in TaskCard for "Remove from queue" functionality
- **Agent stopping via `useStopAgent`**: Already exists in `specTaskWorkflowService.ts`
- **Menu actions in TaskCard**: Existing pattern using MUI `Menu` and `MenuItem` components

### Approach

Combine two existing operations:
1. Stop the running agent (if any) using `useStopAgent`
2. Update task status to `backlog` using `useUpdateSpecTask`

### UI Location

Add "Move to Backlog" option to the existing TaskCard overflow menu (three-dot menu), similar to the existing "Remove from queue" option. Also expose in SpecTaskDetailContent toolbar for consistency.

### Eligible Statuses

Tasks can be moved back to backlog from these statuses:
- `spec_generation` (planning in progress)
- `spec_review` (awaiting review)
- `spec_revision` (revision requested)
- `spec_approved` (approved, waiting for implementation)
- `implementation` (implementation in progress)
- `implementation_review` (code review)

Not eligible:
- `backlog` (already there)
- `queued_*` (use existing "Remove from queue" action)
- `done` (completed work shouldn't be reverted)
- `pull_request` (PR is open externally)
- `*_failed` (can use retry or move to backlog)

## Key Decisions

1. **Stop agent first, then update status**: This ensures clean shutdown before state change
2. **Use existing hooks**: No new API endpoints needed—`useStopAgent` + `useUpdateSpecTask` cover it
3. **Menu placement**: Add to overflow menu to avoid cluttering the main action buttons area
4. **Preserve generated content**: Design docs and any generated code remain (stored in git branch), but task effectively restarts if re-started later

## Components Modified

- `TaskCard.tsx` — Add menu item for phases other than backlog/queued/done/pull_request
- `SpecTaskActionButtons.tsx` or `SpecTaskDetailContent.tsx` — Add button/menu option in detail view