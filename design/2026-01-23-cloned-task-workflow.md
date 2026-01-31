# Cloned Task Workflow Design

**Date:** 2026-01-23
**Status:** Draft
**Author:** Luke (with Claude)

## Problem Statement

When a task is cloned from a completed task to a new project, the current behavior jumps straight into implementation. This is problematic because:

1. **tasks.md has all items ticked** - The cloned spec contains completed checkboxes from the source task, but these tasks need to be done again
2. **Learned information is in design/requirements but not obvious** - During the first task, the agent may have learned things (e.g., "brand color is #3B82F6") and added them to design.md or requirements.md, but tasks.md still says "Ask user for brand color"
3. **Repo differences aren't accounted for** - The target repo may have different file structures, naming conventions, or existing code that requires spec adjustments

## Current Behavior

```
Clone Task
    ↓
Create task with copied specs (requirements.md, design.md, tasks.md)
    ↓
Start implementation immediately (spec_approved → implementation)
    ↓
Agent sees ticked tasks.md and learned info scattered across files
```

## Proposed Behavior (Demo Version)

For the demo, we still go straight into implementation, but we provide a smarter prompt that tells the agent to:

1. **Re-read the spec files** - The agent should first read requirements.md and design.md to absorb any learned information
2. **Reset and adapt tasks.md** - Untick all items and adjust based on:
   - What was learned (e.g., remove "ask user for color" if color is now in design.md)
   - Differences in the target repo
3. **Then implement** - Work through the updated tasks

### Prompt for Cloned Task Implementation

```markdown
## CLONED TASK - Read Specs First

This task was cloned from a completed task in another project. The specs contain learnings from the original implementation.

**Before you start implementing, you MUST:**

1. **Read design.md and requirements.md carefully** - They may contain information that was discovered during the original implementation (decisions made, values confirmed with user, approaches that worked). Use this information instead of re-asking or re-discovering it.

2. **Reset and adapt tasks.md:**
   - All checkboxes are currently marked [x] complete from the original task
   - Change all [x] back to [ ] (unchecked)
   - REMOVE any tasks that are no longer needed based on what you learned from reading the specs
   - ADD any new tasks specific to this repository if needed
   - Push the updated tasks.md to helix-specs BEFORE doing any implementation work

3. **Adapt to this repository** - The target repo may differ from the original:
   - Check file paths and structure
   - Verify naming conventions match
   - Look for existing code that might change the approach

The clone feature's value is that learnings transfer - you should NOT need to re-ask questions or re-discover things the original agent already figured out.
```

## Future Behavior (Post-Demo)

For production use with potentially larger repo differences, we should consider:

### Option A: Cloned Planning Phase

```
Clone Task
    ↓
Create task with copied specs
    ↓
"Cloned Spec Review" phase (new status: spec_cloned_review)
    ↓
Agent reads original specs + explores new repo
    ↓
Agent produces adapted specs for the new repo
    ↓
User reviews adapted specs
    ↓
Implementation
```

**Pros:**
- Handles major repo differences
- User can verify adaptations before implementation
- Clean separation of concerns

**Cons:**
- Slower for simple cases (like brand color demo)
- More phases to track

### Option B: Smart Auto-Adaptation

```
Clone Task
    ↓
Create task with copied specs
    ↓
Agent automatically adapts specs (background, no user review)
    ↓
Implementation with adapted specs
```

**Pros:**
- Fast like current approach
- Handles adaptations automatically

**Cons:**
- No user oversight of adaptations
- May make wrong assumptions

### Recommendation

For now (demo): Use **current implementation + smarter prompt** (described above)

For future: Implement **Option A** with a user toggle:
- "Quick clone" → straight to implementation with smart prompt
- "Full clone" → cloned planning phase with user review

## Implementation Notes

### Files to Modify

1. **api/pkg/services/agent_instruction_service.go** - Add new `BuildClonedTaskImplementationPrompt` function
2. **api/pkg/server/spec_task_clone_handlers.go** - Use the cloned prompt when starting cloned tasks
3. **api/pkg/types/simple_spec_task.go** - Possibly add `spec_cloned_review` status for future

### Detecting Cloned Tasks

Tasks already have `cloned_from_id` field. When starting implementation on a task where `cloned_from_id != ""`, use the cloned task prompt instead of the standard approval prompt.

### Tasks.md Reset Logic

The prompt tells the agent to reset tasks.md, but we could also:
- Pre-process tasks.md on clone to untick all items
- Add a marker comment like `<!-- CLONED: Adapt these tasks for this repo -->`

## Demo Impact

For the Brand Color Demo, this means:

**Before (current):**
- Agent sees tasks.md with "[x] Ask user for brand color"
- Agent might ask again or get confused

**After (with smarter prompt):**
- Agent reads design.md, sees "Brand color: #3B82F6"
- Agent removes the "ask for color" task from tasks.md
- Agent just fills the shape with the learned color

This demonstrates the clone feature's value: **learn once, apply many times**.

## Open Questions

1. Should we pre-process tasks.md on clone (untick all) or let the agent do it?
2. For major repo differences, should we ever re-run the full planning phase?
3. Should there be a "diff view" showing what changed between original and adapted specs?
