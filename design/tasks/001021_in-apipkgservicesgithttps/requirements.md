# Requirements: Optimistic Concurrency Control for Git Push

## Overview

Implement optimistic concurrency control in the git HTTP server's receive-pack handler to prevent agents from accidentally overwriting external changes to upstream branches.

## User Stories

### US1: Prevent Overwriting External Changes
**As a** user with external contributors pushing to the same repository  
**I want** Helix agents to detect when upstream has changed externally  
**So that** agent pushes don't silently overwrite human commits

### US2: Clear Error Messages
**As an** agent receiving a push rejection  
**I want** a clear error message explaining why the push failed  
**So that** I know to pull the latest changes before retrying

### US3: Allow Agent-Only Branch Updates
**As a** Helix agent working on a feature branch  
**I want** to be able to force-push my own changes  
**So that** I can amend commits and rebase without conflicts when I'm the only contributor

## Acceptance Criteria

### AC1: Upstream Divergence Detection
- [ ] Before pushing to upstream, fetch the current HEAD of the target branch
- [ ] Compare upstream HEAD to the `old_ref` (what middle repo had before agent's push)
- [ ] If they differ, upstream changed externally since last sync

### AC2: Push Rejection on Divergence
- [ ] When upstream diverged, reject the push by rolling back the branch to `old_ref`
- [ ] Return error: "Push rejected: upstream branch changed externally. Pull latest changes and retry."
- [ ] Do NOT push to upstream when divergence is detected

### AC3: Allow Normal Force-Push
- [ ] When upstream HEAD matches `old_ref`, allow the push (including force-pushes)
- [ ] This permits agents to amend/rebase when they're the only ones working on the branch

### AC4: Logging
- [ ] Log upstream fetch and comparison results at Debug level
- [ ] Log rejection events at Warn level with branch, old_ref, and upstream HEAD