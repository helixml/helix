# Design: Primary Repository Highlighting in Spectask Prompts

## Overview

Modify the planning and implementation phase prompts to clearly indicate which repository is the PRIMARY target for work, and which repositories are attached for reference or dependent changes.

## Current State

- **Planning prompt** (`spec_task_prompts.go`): Lists repos generically as "Code repos (don't touch these)"
- **Orchestrator prompt** (`spec_task_orchestrator.go`): Lists "Available Repositories" without distinguishing primary
- **Implementation prompt** (`agent_instruction_service.go`): Mentions `PrimaryRepoName` but only deep in the prompt

The `DefaultRepoID` field on projects already identifies the primary repo. The `HELIX_PRIMARY_REPO_NAME` env var is already set in containers. We just need to surface this info prominently in the prompts.

## Design Decisions

### 1. Add Primary Repo Info to Prompt Data Structures

Add `PrimaryRepoName` parameter to `BuildPlanningPrompt()` to match how `BuildApprovalInstructionPrompt()` already works.

### 2. Prominent Multi-Repo Section in Both Prompts

Add a new section early in both planning and implementation prompts:

```markdown
## Repository Context

**PRIMARY REPOSITORY (where your main work happens):**
- `website` at `/home/retro/work/website/`

**Reference/Dependent Repositories (for context or dependent changes only):**
- `launchpad` at `/home/retro/work/launchpad/`
- `shared-lib` at `/home/retro/work/shared-lib/`

Your main changes should go to the PRIMARY repository. Only modify reference repos if the task explicitly requires dependent changes.
```

### 3. When No Repos or Single Repo

- **No repos:** Don't show the section
- **Single repo:** Show it as PRIMARY, no reference section
- **Multiple repos:** Show primary + reference list

## Files to Modify

1. **`api/pkg/services/spec_task_prompts.go`**
   - Add `PrimaryRepoName` and `AllRepos` to `PlanningPromptData`
   - Add repository context section to `planningPromptTemplate`
   - Update `BuildPlanningPrompt()` signature to accept repo info

2. **`api/pkg/services/spec_task_orchestrator.go`**
   - Update `buildPlanningPrompt()` to identify primary repo and pass to template
   - Use `project.DefaultRepoID` to determine which repo is primary

3. **`api/pkg/services/spec_driven_task_service.go`**
   - Update `StartSpecGeneration()` call to `BuildPlanningPrompt()` with repo info

4. **`api/pkg/services/agent_instruction_service.go`**
   - Move the repository context section earlier in `approvalPromptTemplate`
   - Add reference repos list (currently only shows primary)

## Data Flow

```
Project.DefaultRepoID
        ↓
Store.ListGitRepositories(projectID)
        ↓
Identify which repo.ID == project.DefaultRepoID
        ↓
Pass (primaryRepoName, allRepos) to BuildPlanningPrompt/BuildApprovalPrompt
        ↓
Template renders prominent repository context section
```

## Testing

- Verify prompts with 0, 1, and 3 repos
- Verify primary repo is correctly identified
- Manually test that agents understand the distinction