# Clone Feature Improvements: Session Context + Worktree Attachment

**Date:** 2026-01-14
**Author:** Claude
**Status:** Draft for Review

## Problem Statement

The current clone feature copies spec task metadata (prompt, requirements, technical design, implementation plan) to new projects but doesn't carry forward the *learning* from the source session. When a user runs a complex task in one project and wants to apply the same solution to 10 other repositories, the cloned tasks:

1. Start fresh with no knowledge of how the original agent solved the problem
2. Can't reference what worked/didn't work in the source implementation
3. Can't access the actual code changes made in the source branch

This means agents solving cloned tasks re-discover solutions rather than applying known patterns.

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
    ClonedFromID:        source.ID,        // Track lineage
    ClonedFromProjectID: source.ProjectID,
    CloneGroupID:        cloneGroupID,
}
```

**Not cloned:**
- `PlanningSessionID` - The conversation history with all context
- `ExternalAgentID` - The agent state
- `BranchName` - The git branch with actual code changes
- Work sessions, Zed threads, etc.

## Proposed Improvements

### Improvement 1: Session Context via MCP Tools

We already have a Session MCP Server (`api/pkg/session/mcp_server.go`) that provides tools for agents to navigate conversation history:

- `current_session` - Quick overview
- `session_toc` - Table of contents with numbered turns
- `session_title_history` - Topic evolution
- `search_session` - Search within a session
- `get_turn` / `get_turns` - Retrieve specific content

**Proposal:** When a cloned task starts, inject the source session context:

1. **Store source session reference in cloned task:**
   ```go
   newTask := &types.SpecTask{
       // ... existing fields ...
       SourceSessionID: source.PlanningSessionID,  // NEW
   }
   ```

2. **Generate session summary at clone time:**
   - Call `/sessions/{id}/toc` on the source session
   - Call `/sessions/{id}/summary` (new endpoint, see below) to get a compressed narrative
   - Store in `CloneGroup.SourceSessionSummary`

3. **Inject context into cloned task's initial prompt:**
   ```
   This task is cloned from a previous implementation. Here's what the original agent learned:

   ## Session Summary
   [Compressed summary of key decisions, problems encountered, solutions that worked]

   ## Reference
   You can access the full source session via MCP tools:
   - session_toc(session_id="ses_xxx") to see the table of contents
   - get_turn(session_id="ses_xxx", turn=N) to read specific interactions
   ```

4. **New API endpoint:** `GET /api/v1/sessions/{id}/summary`
   - Uses LLM to generate a compressed narrative from the TOC + key turns
   - Focuses on: decisions made, problems encountered, solutions that worked
   - Cacheable (store in session metadata after first generation)

### Improvement 2: Source Branch as Git Worktree

The source task's branch contains actual code changes that solved the problem. Making this available as a reference would be valuable.

**Proposal:** Attach source branch as a separate worktree in the cloned task's workspace:

1. **At desktop container startup:**
   - If task has `ClonedFromID` with a `BranchName`:
   - Create a git worktree in a read-only reference directory:
     ```bash
     git worktree add /project/reference/source-impl origin/${source_branch} --detach
     ```

2. **Desktop bridge configuration:**
   ```go
   type WorkspaceConfig struct {
       ReferenceWorktrees []ReferenceWorktree `json:"reference_worktrees,omitempty"`
   }

   type ReferenceWorktree struct {
       Path       string `json:"path"`        // e.g., "/project/reference/source-impl"
       Branch     string `json:"branch"`      // e.g., "feature/add-dark-mode-task-42"
       SourceTask string `json:"source_task"` // e.g., "sptsk_abc123"
       ReadOnly   bool   `json:"read_only"`   // true for reference
   }
   ```

3. **Agent system prompt addition:**
   ```
   A reference implementation is available at /project/reference/source-impl/
   This contains the code changes from the original task. You can:
   - Read files to understand the pattern used
   - Diff to see what was changed
   - Adapt the approach for this repository
   ```

4. **Safeguards:**
   - Read-only mount or ownership prevents accidental modifications
   - Clear labeling in Zed file browser ("Reference: source-impl (read-only)")
   - Worktree is detached HEAD (not tracking any branch)

### Improvement 3: Clone Group Dashboard Enhancements

Since we're adding session context, enhance the clone group tracking:

1. **Add to CloneGroup type:**
   ```go
   type CloneGroup struct {
       // ... existing fields ...
       SourceSessionID      string `json:"source_session_id,omitempty"`
       SourceSessionSummary string `json:"source_session_summary,omitempty" gorm:"type:text"`
       SourceBranchName     string `json:"source_branch_name,omitempty"`
   }
   ```

2. **Clone group progress view shows:**
   - Link to view source session
   - Button to regenerate session summary
   - Diff view comparing cloned task branches to source

## Implementation Plan

### Phase 1: Session Summary API
1. Add `GET /api/v1/sessions/{id}/summary` endpoint
2. Implement LLM-based summarization using existing TOC
3. Cache summary in session metadata
4. Add `summary` field to session MCP tools response

### Phase 2: Clone Context Injection
1. Add `SourceSessionID` and `SourceBranchName` to SpecTask type
2. Modify `cloneTaskToProject()` to populate these fields
3. Store session summary in CloneGroup
4. Modify task start to inject context into initial prompt

### Phase 3: Reference Worktree
1. Add `WorkspaceConfig.ReferenceWorktrees` type
2. Modify Hydra desktop container startup to create worktrees
3. Add read-only mount for reference directories
4. Update Zed config to show reference worktrees in file browser

### Phase 4: UI Enhancements
1. Clone dialog shows session preview option
2. Clone group progress shows source session link
3. Active task shows "Reference implementation available" indicator

## API Changes

### New Endpoint: Session Summary
```
GET /api/v1/sessions/{id}/summary
Response:
{
    "session_id": "ses_xxx",
    "session_name": "Add dark mode to settings",
    "summary": "Implemented dark mode toggle...",
    "key_decisions": [
        "Used CSS custom properties for theme switching",
        "Added context provider for theme state"
    ],
    "problems_solved": [
        "Fixed flash of unstyled content on page load"
    ],
    "generated_at": "2026-01-14T10:30:00Z"
}
```

### Modified Clone Request
```
POST /api/v1/spec-tasks/{taskId}/clone
{
    "target_project_ids": ["prj_xxx"],
    "auto_start": true,
    "include_session_context": true,  // NEW: default true
    "include_reference_worktree": true // NEW: default true
}
```

## Security Considerations

1. **Session access:** Only clone session context if user has access to both source and target projects
2. **Branch access:** Verify user has read access to source repository before creating worktree
3. **Isolation:** Reference worktrees are read-only and don't affect target repository
4. **Summary privacy:** Session summary is generated from user's own data, stored per clone group

## Metrics

Track effectiveness:
- Time to completion: cloned tasks vs fresh tasks
- Agent iteration count: how many attempts before success
- User intervention rate: human corrections needed

## Open Questions

1. **Summary regeneration:** Should we allow manual triggering if source session grows?
2. **Worktree conflicts:** What if source branch doesn't exist in target repo (different repo)?
3. **Storage:** How long to keep reference worktrees? Clean up after task completion?

## Alternatives Considered

### Alternative 1: Full Session Clone
Clone the entire session to the new task, giving it complete access to all conversation history.

**Rejected:** Sessions can be very large (hundreds of turns). This would bloat storage and slow down agent context loading. The summary approach provides 80% of the value at 5% of the cost.

### Alternative 2: RAG-based Session Search
Index session content and provide semantic search to cloned tasks.

**Deferred:** The MCP tools already provide search functionality. RAG would be an enhancement on top of this proposal, not a replacement.

### Alternative 3: Code Snippet Injection
Extract key code changes from source branch and inject as examples in the prompt.

**Partially adopted:** The session summary captures high-level decisions. The reference worktree provides access to actual code. Together they cover this use case better than extracted snippets.

## Conclusion

These improvements transform the clone feature from "copy the specification" to "learn once, apply many times." By combining session context (what the agent learned) with reference worktrees (what code was written), cloned tasks start with a significant head start rather than from scratch.

The MCP session tools we already built provide the foundation - this proposal connects them to the clone workflow.
