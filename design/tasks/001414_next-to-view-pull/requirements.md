# Requirements: Branch Sync Status & Auto-Merge Feature

## Overview

Show branch divergence status next to the "View Pull Request" button on TaskCard, with an option to automatically merge the default branch into the feature branch via the agent.

## User Stories

### US-1: View Branch Status
**As a** user reviewing a task in pull_request status  
**I want to** see if my branch is behind the default branch  
**So that** I know if merge conflicts might occur before merging

### US-2: Auto-Merge Default Branch
**As a** user with a branch that's behind  
**I want to** click a button to have the agent merge the default branch into my feature branch  
**So that** I don't have to manually resolve the sync

### US-3: Conflict Handling
**As a** user whose branch has merge conflicts  
**I want to** be informed when the agent needs help resolving a conflict  
**So that** I can provide guidance or resolve it manually

## Acceptance Criteria

### AC-1: Branch Status Display
- [ ] When task is in `pull_request` status, show commits behind count next to "View Pull Request" button
- [ ] Display "up to date" indicator when branch is not behind
- [ ] Display "X commits behind main" when branch is behind default branch
- [ ] Status should auto-refresh when task data updates

### AC-2: Merge Button
- [ ] Show "Sync Branch" button when branch is behind
- [ ] Button triggers agent to merge default branch into feature branch
- [ ] Button is disabled while merge is in progress
- [ ] Show loading state during merge operation

### AC-3: Agent Prompt
- [ ] New prompt template instructs agent to merge default branch
- [ ] Agent pushes result after successful merge
- [ ] Agent reports back if conflicts need user intervention

### AC-4: Conflict Notification
- [ ] If merge fails due to conflicts, agent sends message explaining the situation
- [ ] UI shows conflict state so user knows action is needed