# Design: Spec Task Description Edit Bug

## Overview

Fix the backend to use the user-edited `Description` field instead of `OriginalPrompt` when starting spec tasks.

## Architecture

No architectural changes required. This is a simple field substitution in two locations.

## Key Decision

**Use `Description` as the source of truth for agent prompts, keep `OriginalPrompt` for audit history.**

Rationale:
- `Description` is the user-facing, editable field that users expect to control
- `OriginalPrompt` serves as an immutable audit trail of what was originally requested
- Minimal code change with clear semantics

## Implementation Approach

### Backend Changes (`api/pkg/services/spec_driven_task_service.go`)

**Change 1: StartSpecGeneration() ~line 400**
```go
// Before:
fullMessage = planningPrompt + "\n\n**User Request:**\n" + task.OriginalPrompt

// After:
fullMessage = planningPrompt + "\n\n**User Request:**\n" + task.Description
```

**Change 2: StartJustDoItMode() ~line 637**
Same pattern - use `task.Description` instead of `task.OriginalPrompt` in the log statement and prompt building.

### Cloned Task Handling

For cloned tasks (line ~400), the behavior should also use `Description`:
```go
// Before:
fullMessage = planningPrompt + "\n\n**Original Request (for context only...):**\n> \"" + task.OriginalPrompt + "\""

// After:
fullMessage = planningPrompt + "\n\n**Original Request (for context only...):**\n> \"" + task.Description + "\""
```

## Edge Cases

1. **Description is empty**: If user clears Description, fall back to OriginalPrompt
   ```go
   prompt := task.Description
   if prompt == "" {
       prompt = task.OriginalPrompt
   }
   ```

2. **Existing tasks**: Both fields are identical for tasks where user never edited, so behavior unchanged

## Testing

1. Create a spec task with description "Build feature A"
2. Edit description to "Build feature B with extra requirements"  
3. Start planning
4. Verify agent receives "Build feature B with extra requirements"
5. Repeat for Just Do It mode

## Implementation Notes

**Changes made:**

1. `StartSpecGeneration()` (~line 396-404): Added `userPrompt` variable with fallback logic, used in both normal and cloned task cases
2. `StartJustDoItMode()` (~line 643-649): Added same `userPrompt` variable pattern with fallback logic
3. Also updated log statement in `StartJustDoItMode()` to log `user_prompt` instead of `original_prompt`

**Pattern used:**
```go
// Use Description (user-editable) with fallback to OriginalPrompt (immutable original)
userPrompt := task.Description
if userPrompt == "" {
    userPrompt = task.OriginalPrompt
}
```

**Build verification:** `CGO_ENABLED=0 go build -o /tmp/helix-bin .` passes

**Note:** The API hot-reloads via Air, so changes take effect without restart.