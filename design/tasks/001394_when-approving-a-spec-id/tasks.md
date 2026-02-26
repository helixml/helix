# Implementation Tasks

- [ ] Add `ApprovalComments` field to `ApprovalPromptData` struct in `api/pkg/services/agent_instruction_service.go`
- [ ] Update `approvalPromptTemplate` to include conditional "Reviewer's Note" section
- [ ] Update `BuildApprovalInstructionPrompt` function signature to accept `approvalComments` parameter
- [ ] Update `SendApprovalInstruction` to extract and pass approval comments (filtering out default "Design approved")
- [ ] Update `sendApprovalInstructionToAgent` in `api/pkg/server/spec_task_design_review_handlers.go` to pass approval comments
- [ ] Test: approve a spec with a custom message and verify agent receives it
- [ ] Test: approve a spec with default message and verify no "Reviewer's Note" section appears