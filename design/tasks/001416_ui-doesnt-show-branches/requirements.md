# Requirements: Multi-Repository PR Support

## Problem Statement

When a project has multiple repositories attached, the system only tracks and displays PRs for the primary repository. Branches pushed to non-primary repos are not surfaced in the UI, and PRs may not be automatically created for them.

## Constraints

**Branch Naming**: The Git system enforces that agents can only push to branches matching the pre-ordained branch name for their spec task (e.g., `feature/001234_task-name`). This branch name is consistent across all repositories in the project. This constraint simplifies PR tracking since we can identify task branches by name in any repo.

## User Stories

### US-1: View PRs Across All Project Repositories
**As a** developer working on a multi-repo project  
**I want to** see all open PRs for my task across all attached repositories  
**So that** I can track the full scope of my changes and ensure nothing is missed

### US-2: Automatic PR Creation for All Repos
**As a** developer  
**I want** PRs to be automatically created in all repositories where my task branch was pushed  
**So that** I don't have to manually create PRs in each repo

## Acceptance Criteria

### AC-1: UI Shows Multiple PRs
- [ ] Task detail view displays a list of PRs (one per repo with changes)
- [ ] Each PR entry shows: repo name, PR number, PR title, PR URL
- [ ] Empty repos (no branch pushed) are not shown

### AC-2: PR Auto-Creation for Non-Primary Repos
- [ ] When branch is pushed to any external repo, system checks if PR exists
- [ ] PR is created if branch exists and no open PR for that branch
- [ ] PR creation follows same logic as primary repo (title, description format)

### AC-3: Data Model Updates
- [ ] SpecTask stores list of per-repo PR info, not just single PR
- [ ] Existing single-PR field remains for backward compatibility
- [ ] API returns full list of PRs for a task

## Out of Scope
- Cross-repo dependency management
- PR merge orchestration across repos
- Branch sync between repos