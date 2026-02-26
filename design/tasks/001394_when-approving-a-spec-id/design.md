# Design: Include Approval Message in Agent Instruction

## Problem

The approval flow stores the reviewer's comment but doesn't pass it to the agent. The rejection flow correctly passes comments using `BuildRevisionInstructionPrompt(t, comments)`, but the approval flow omits them.

## Solution

Add the approval comment to `BuildApprovalInstructionPrompt` and include it in the approval template.

## Code Changes

### 1. Update `ApprovalPromptData` struct

**File:** `api/pkg/services/agent_instruction_service.go`

Add a new field:
```go
type ApprovalPromptData struct {
    // ... existing fields ...
    ApprovalComments string // Reviewer's approval comment (optional)
}
```

### 2. Update `approvalPromptTemplate`

Add a conditional section after the task info:

```go
{{if .ApprovalComments}}

## Reviewer's Note

{{.ApprovalComments}}
{{end}}
```

### 3. Update `BuildApprovalInstructionPrompt` signature and usage

**Current:**
```go
func BuildApprovalInstructionPrompt(task *types.SpecTask, branchName, baseBranch, guidelines, primaryRepoName string) string
```

**New:**
```go
func BuildApprovalInstructionPrompt(task *types.SpecTask, branchName, baseBranch, guidelines, primaryRepoName, approvalComments string) string
```

### 4. Update callers

**In `SendApprovalInstruction`:**
```go
// Get approval comments if available
approvalComments := ""
if task.SpecApproval != nil && task.SpecApproval.Comments != "" && task.SpecApproval.Comments != "Design approved" {
    approvalComments = task.SpecApproval.Comments
}
message := BuildApprovalInstructionPrompt(task, branchName, baseBranch, guidelines, primaryRepoName, approvalComments)
```

**In `sendApprovalInstructionToAgent` (spec_task_design_review_handlers.go):**
Same pattern - pass approval comments to `BuildApprovalInstructionPrompt`.

## Design Decisions

1. **Filter default "Design approved" message** - The frontend sends this as a default. Showing it to the agent adds no value, so we filter it out.

2. **Label as "Reviewer's Note"** - Clear labeling helps the agent understand this is human context, not a system instruction.

3. **Add to end of template** - The comment appears after the task details but before implementation starts, giving context without interrupting the instruction flow.

## Files to Modify

1. `api/pkg/services/agent_instruction_service.go` - struct, template, and `BuildApprovalInstructionPrompt`
2. `api/pkg/server/spec_task_design_review_handlers.go` - update call to `BuildApprovalInstructionPrompt`
