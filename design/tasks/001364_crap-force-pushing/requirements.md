# Requirements: Force Push Handling in Git Integration

## Problem Statement

When someone force-pushes to an external repository (e.g., GitHub), the Helix git integration breaks. The "middle repo" (Helix's cached bare repository) becomes diverged from upstream, causing:

1. Agent pushes fail with non-fast-forward errors
2. Sync operations may silently drop agent commits
3. The system gets into an inconsistent state

## User Stories

### US1: External Force Push Recovery
**As a** developer who force-pushed to GitHub  
**I want** Helix to automatically recover and sync  
**So that** agents can continue working without manual intervention

### US2: Agent Push After Force Push
**As an** agent working on a task  
**I want** my commits to succeed even if someone force-pushed upstream  
**So that** my work isn't lost

### US3: Conflict Notification
**As a** user  
**I want** to be notified when force-push divergence is detected  
**So that** I understand why a sync happened

## Acceptance Criteria

1. **Force Sync on Divergence**: When `SyncAllBranches` detects the middle repo has commits not in upstream (force-push happened), it should:
   - Log a warning about detected force-push
   - Force-update local refs to match upstream
   - NOT lose any agent commits that haven't been pushed yet

2. **Agent Push Recovery**: When an agent's push to upstream fails due to non-fast-forward:
   - Detect this specific error case
   - Re-sync from upstream with force
   - Rebase/replay agent commits on new upstream HEAD
   - Retry the push

3. **Pre-Push Sync**: Before accepting agent pushes via `handleReceivePack`:
   - Already syncs from upstream (existing behavior)
   - Should handle the case where sync reveals upstream was force-pushed
   - Ensure agent's push will succeed if their changes are rebased

4. **No Data Loss**: Agent commits must never be silently dropped. Either:
   - Successfully push to upstream, OR
   - Fail with clear error message, OR
   - Store orphaned commits for manual recovery

## Out of Scope

- Preventing external force pushes (that's the user's choice)
- Three-way merge conflict resolution (just rebase/fast-forward)
- UI for resolving complex divergence scenarios