# SpecTask Branch Selection UX Design

**Date:** 2025-12-16
**Status:** Proposed
**Author:** Claude (with Luke)

## Overview

Add branch configuration to SpecTask creation that allows users to:
1. Start a new feature branch from any base branch
2. Continue work on an existing branch

The design mirrors familiar GitHub/GitLab workflows while providing full flexibility.

## User Requirements

Three use cases to support:
1. **New branch from main** — Starting a new feature (most common)
2. **New branch from another branch** — Branching from an existing feature branch
3. **Continue existing branch** — Resume work on a branch that already has commits

## UI Design

### Location

Add a new section in the existing "New SpecTask" right panel, positioned **after** the prompt text area and **before** the agent selection.

### Primary Selection: Two Radio Options

```
┌─────────────────────────────────────────────────────────────────┐
│  Where do you want to work?                                      │
│                                                                   │
│  ┌─────────────────────────┐  ┌─────────────────────────────────┐│
│  │ ● Start fresh           │  │ ○ Continue existing work        ││
│  │                         │  │                                 ││
│  │   Create a new branch   │  │   Resume work on an existing    ││
│  │   from a base           │  │   branch                        ││
│  └─────────────────────────┘  └─────────────────────────────────┘│
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

### Option 1: "Start fresh" (Default)

Shows base branch selection and branch name customization:

```
┌─────────────────────────────────────────────────────────────────┐
│  ● Start fresh                                                   │
│                                                                   │
│  Base branch:                                                    │
│  ┌─────────────────────────────────────┐                        │
│  │ main                              ▼ │                        │
│  └─────────────────────────────────────┘                        │
│  [Dropdown contains ALL branches - main is the default]         │
│                                                                   │
│  Branch name:                                                    │
│  ┌─────────────────────────────────────┐                        │
│  │ feature/user-auth                   │                        │
│  └─────────────────────────────────────┘                        │
│  Will create: feature/user-auth-{task#}                         │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

**Behavior:**
- Base branch dropdown defaults to `main` (or project's configured default branch)
- Dropdown contains all branches from attached repositories
- Branch name field is optional — empty defaults to `spec-task`
- Task number is always appended as suffix for uniqueness (e.g., `feature/user-auth-123`)
- Helper text shows the final branch name with `{task#}` placeholder

### Option 2: "Continue existing work"

Shows branch picker for existing branches:

```
┌─────────────────────────────────────────────────────────────────┐
│  ● Continue existing work                                        │
│                                                                   │
│  Select branch:                                                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ 🔍 Search branches...                                        ││
│  ├─────────────────────────────────────────────────────────────┤│
│  │ ⭐ Recent branches                                           ││
│  │   feature/user-auth                                          ││
│  │   fix/login-bug                                              ││
│  │ ──────────────────────────────────────────────────────────── ││
│  │ All branches                                                 ││
│  │   feature/user-auth                                          ││
│  │   feature/api-v2                                             ││
│  │   fix/login-bug                                              ││
│  │   refactor/database                                          ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                   │
│  ⓘ The agent will continue from where this branch left off      │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

**Behavior:**
- Searchable dropdown with recent branches prioritized
- Shows branches from all attached project repositories
- No new branch created — agent checks out existing branch and continues

## Data Model Changes

### Frontend: Extend `TypesCreateTaskRequest`

```typescript
interface TypesCreateTaskRequest {
  // ... existing fields ...

  // NEW: Branch configuration
  branch_mode?: 'new' | 'existing';

  // For 'new' mode:
  base_branch?: string;      // defaults to 'main' or project default
  branch_prefix?: string;    // user-specified prefix, task# appended

  // For 'existing' mode:
  working_branch?: string;   // the branch to continue working on
}
```

### Backend Behavior

**New branch mode (`branch_mode: 'new'`):**
1. Create SpecTask and get task number
2. Generate branch name: `{branch_prefix}-{task_number}` (e.g., `feature/user-auth-123`)
3. Create new branch from `base_branch`
4. Checkout new branch in sandbox

**Existing branch mode (`branch_mode: 'existing'`):**
1. Create SpecTask
2. Checkout `working_branch` in sandbox
3. Agent continues from current HEAD

## Visual Design Notes

**Icons (GitHub-inspired):**
- Start fresh: `GitBranch` icon with `+` badge
- Continue existing: `History` or `GitMerge` icon

**Colors:**
- Start fresh: Secondary/blue (new thing)
- Continue existing: Neutral (existing thing)

## Edge Cases

1. **Multiple repositories:** Show repo prefix for clarity: `helix-api: main`

2. **Empty/uncloned repository:** Disable "Continue existing" with tooltip explaining why

3. **Stale branch data:** Include "Refresh branches" button

4. **Branch protection:** Dim protected branches with explanatory tooltip

5. **Default behavior:** If user skips section entirely, default to "Start fresh from main" with auto-generated branch name

## Implementation Phases

### Phase 1 (MVP)
- Two radio buttons for mode selection
- Base branch dropdown (all branches, main default)
- Branch name text field with suffix preview
- Existing branch dropdown (simple list)

### Phase 2
- Branch search functionality
- Recent branches section
- Repository indicators for multi-repo projects

### Phase 3
- Last commit info display
- Branch protection indicators
- Smart branch name suggestions from task prompt

## API Endpoint Changes

The existing `POST /api/v1/spec-tasks/from-prompt` endpoint needs to accept the new fields:

```go
type CreateTaskRequest struct {
    // ... existing fields ...

    BranchMode    string `json:"branch_mode"`     // "new" or "existing"
    BaseBranch    string `json:"base_branch"`     // for new mode
    BranchPrefix  string `json:"branch_prefix"`   // for new mode
    WorkingBranch string `json:"working_branch"`  // for existing mode
}
```

## Open Questions

1. Should we validate branch names for invalid characters in the frontend?
2. How do we handle conflicts if the generated branch name already exists?
3. Should "Continue existing" show the last commit message/date for context?
