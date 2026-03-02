# Design: Chrome MCP Testing Documentation

## Overview

Add guidance to planning and implementation prompts about using Chrome MCP for bug reproduction and fix verification in web application tasks.

## Files to Modify

| File | Change |
|------|--------|
| `helix/api/pkg/services/spec_task_prompts.go` | Add bug reproduction note to planning prompt |
| `helix/api/pkg/services/agent_instruction_service.go` | Add fix verification note to implementation prompt |

## Changes

### Planning Prompt (`spec_task_prompts.go`)

Add a new subsection under the existing "Visual Testing" section:

```markdown
**For web app bugs/issues:** Before writing specs, attempt to reproduce the bug using Chrome MCP:
1. Navigate to the affected page
2. Perform the steps that trigger the issue
3. Document what you observe (screenshot if helpful)
4. Include reproduction steps in requirements.md

This helps you understand the problem before designing the solution.
```

**Location**: After the existing screenshot workflow, before "Document Your Learnings"

### Implementation Prompt (`agent_instruction_service.go`)

Add a new subsection under the existing "Visual Testing & Screenshots" section:

```markdown
**Testing your fixes:** After implementing changes to a web app:
1. Run/restart the application
2. Use Chrome MCP to navigate to the affected area
3. Verify the fix works as expected
4. Take before/after screenshots as proof

Don't just assume code changes work - verify in the browser when possible.
```

**Location**: After the screenshot workflow, before "Don't Over-Engineer"

## Rationale

- **Minimal change**: Adds 4-6 lines to each prompt, no restructuring
- **Optional guidance**: Uses "attempt to" and "when possible" language - not mandatory
- **Leverages existing infrastructure**: Chrome MCP already available, just under-documented
- **Improves quality**: Agents who reproduce bugs before designing tend to create better specs