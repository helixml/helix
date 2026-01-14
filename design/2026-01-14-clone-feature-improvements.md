# Clone Feature Improvements: Session Context + Reference Repository

**Date:** 2026-01-14
**Author:** Claude
**Status:** Draft for Review

## Problem Statement

The current clone feature copies spec task metadata (prompt, requirements, technical design, implementation plan) to new projects but doesn't carry forward the *learning* from the source session. When a user runs a complex task in one project and wants to apply the same solution to 10 other repositories, the cloned tasks:

1. Start fresh with no knowledge of how the original agent solved the problem
2. Can't reference what worked/didn't work in the source implementation
3. Can't access the actual code changes made in the source branch

This means agents solving cloned tasks re-discover solutions rather than applying known patterns.

## Demo: Data Pipeline Logging Migration

We have a working sample project (`clone-demo-pipelines`) with 5 data pipeline projects:
- Stocks (Start Here) - has the logging task
- Bonds, Forex, Options, Indicators - clone targets

The task is to add structured logging with correlation ID tracing. The agent learns the `contextvars` pattern for async context propagation on Stocks, then that knowledge should transfer to the other 4 pipelines.

**Current problem:** When cloning, the specs transfer but not the session learning.

## MVP Approach: Session TOC Injection

The simplest improvement uses infrastructure we already have.

### What We Already Have

1. **Session MCP Server** (`api/pkg/session/mcp_server.go`) with tools:
   - `session_toc(session_id)` - Table of contents
   - `get_turn(session_id, turn)` - Retrieve specific content
   - `search_session(session_id, query)` - Search within session

2. **TOC API Endpoint** (`GET /api/v1/sessions/{id}/toc`) - Returns formatted TOC

3. **Clone tracking fields** - `ClonedFromID`, `ClonedFromProjectID`, `CloneGroupID`

### MVP Implementation

**Step 1: Add source context fields to SpecTask**

```go
// In types/simple_spec_task.go
type SpecTask struct {
    // ... existing clone tracking ...
    ClonedFromID        string `json:"cloned_from_id,omitempty"`
    ClonedFromProjectID string `json:"cloned_from_project_id,omitempty"`
    CloneGroupID        string `json:"clone_group_id,omitempty"`

    // NEW: Source session context for cloned tasks
    SourceSessionID   string `json:"source_session_id,omitempty" gorm:"size:255"`
    SourceBranchName  string `json:"source_branch_name,omitempty" gorm:"size:255"`
    SourceRepoID      string `json:"source_repo_id,omitempty" gorm:"size:255"`
}
```

**Step 2: Populate source context when cloning**

```go
// In spec_task_clone_handlers.go cloneTaskToProject()
newTask := &types.SpecTask{
    // ... existing fields ...
    ClonedFromID:        source.ID,
    ClonedFromProjectID: source.ProjectID,
    CloneGroupID:        cloneGroupID,

    // NEW: Capture source session/branch for context injection
    SourceSessionID:  source.PlanningSessionID,
    SourceBranchName: source.BranchName,
    SourceRepoID:     primaryRepoID,  // From source project
}
```

**Step 3: Inject session TOC into initial prompt**

```go
// In spec_driven_task_service.go StartSpecGeneration()
func (s *SpecDrivenTaskService) buildCloneContext(ctx context.Context, task *types.SpecTask) string {
    if task.SourceSessionID == "" {
        return ""
    }

    // Fetch TOC from source session
    toc, err := s.getSessionTOC(ctx, task.SourceSessionID)
    if err != nil {
        log.Warn().Err(err).Msg("Failed to fetch source session TOC")
        return ""
    }

    return fmt.Sprintf(`
## Context from Previous Implementation

This task was cloned from a completed implementation. Here's what the original agent learned:

### Session Table of Contents
%s

### How to Access Full Details
Use the MCP session tools to dive deeper:
- session_toc(session_id="%s") - Full table of contents
- get_turn(session_id="%s", turn=N) - Read specific interaction
- search_session(session_id="%s", query="...") - Find relevant content

Adapt the patterns learned in the source implementation to this repository.
`, toc.Formatted, task.SourceSessionID, task.SourceSessionID, task.SourceSessionID)
}
```

**Step 4: Prepend to original prompt**

```go
// In StartSpecGeneration, before creating the interaction
cloneContext := s.buildCloneContext(ctx, task)
fullMessage := planningPrompt + "\n\n" + cloneContext + "**User Request:**\n" + task.OriginalPrompt
```

### Reference Repository (Phase 2)

If the source repo exists and has the branch, inject it as an additional repository.

**In spec_task_orchestrator_handlers.go when building DesktopAgent:**

```go
repositoryIDs := []string{}  // Start with project's repos

// Add source repo as reference if this is a cloned task with a branch
if task.SourceRepoID != "" && task.SourceBranchName != "" {
    // Verify source branch still exists
    sourceRepo, err := apiServer.Store.GetGitRepository(ctx, task.SourceRepoID)
    if err == nil && branchExists(sourceRepo.LocalPath, task.SourceBranchName) {
        repositoryIDs = append(repositoryIDs, task.SourceRepoID)

        // Configure reference checkout
        agentReq.ReferenceRepos = []ReferenceRepo{{
            RepoID:   task.SourceRepoID,
            Branch:   task.SourceBranchName,
            Path:     "/project/reference/source-impl",
            ReadOnly: true,
        }}
    }
}
```

**Agent prompt addition:**

```
A reference implementation is available at /project/reference/source-impl/
This contains the actual code changes from the original task on the ${source_branch} branch.
You can read files and diffs to understand the pattern used, then adapt it for this repository.
```

## Implementation Checklist

### Phase 1: Session Context (MVP)
- [ ] Add `SourceSessionID`, `SourceBranchName`, `SourceRepoID` to SpecTask type
- [ ] Run AutoMigrate to add columns
- [ ] Modify `cloneTaskToProject()` to populate source fields
- [ ] Add `buildCloneContext()` helper in spec_driven_task_service.go
- [ ] Modify `StartSpecGeneration()` to inject clone context
- [ ] Test with clone-demo-pipelines sample project

### Phase 2: Reference Repository
- [ ] Add `ReferenceRepos` field to DesktopAgent type
- [ ] Modify desktop startup to checkout reference repos
- [ ] Configure read-only mount for reference path
- [ ] Update agent prompt with reference location
- [ ] Test with clone-demo-pipelines (Stocks branch → Bonds reference)

## Testing with Clone Demo

1. Fork the "Data Pipeline Logging Migration" sample project
2. Complete the logging task on Stocks pipeline
3. Clone to Bonds pipeline
4. Verify:
   - Cloned task has `SourceSessionID` set
   - Initial prompt includes session TOC
   - Agent can call MCP tools to access source session
   - (Phase 2) Source branch is available as reference

## Current Clone Behavior

From `api/pkg/server/spec_task_clone_handlers.go`:

```go
// What gets cloned:
newTask := &types.SpecTask{
    Name:                source.Name,
    OriginalPrompt:      source.OriginalPrompt,
    RequirementsSpec:    source.RequirementsSpec,
    TechnicalDesign:     source.TechnicalDesign,
    ImplementationPlan:  source.ImplementationPlan,
    JustDoItMode:        source.JustDoItMode,
    UseHostDocker:       source.UseHostDocker,
    ClonedFromID:        source.ID,
    ClonedFromProjectID: source.ProjectID,
    CloneGroupID:        cloneGroupID,
}
```

**Not cloned (to be added):**
- `PlanningSessionID` → captured as `SourceSessionID`
- `BranchName` → captured as `SourceBranchName`
- Primary repo ID → captured as `SourceRepoID`

## Conclusion

The MVP is simple: when a cloned task starts, fetch the TOC from the source session and inject it into the prompt along with pointers to the MCP tools. The agent gets context about what was learned and how to dig deeper.

Phase 2 adds the source repository as a reference worktree so the agent can see the actual code changes.

Together, this transforms clone from "copy the specification" to "learn once, apply many."
