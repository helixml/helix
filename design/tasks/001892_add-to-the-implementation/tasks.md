# Implementation Tasks

- [x] Add merge-default-branch step to the `approvalPromptTemplate` Steps section in `agent_instruction_service.go` (use existing `{{.BaseBranch}}` field)
- [~] Add `BaseBranch` field to the template data struct in `helix_code_prompts.go` `ImplementationApprovedPushInstruction()`
- [ ] Update `agent_implementation_approved_push.tmpl` to fetch and merge the default branch before pushing each repo
- [ ] Update the call site in `spec_task_workflow_handlers.go` to pass the default branch to `ImplementationApprovedPushInstruction()`
- [ ] Update the test in `helix_code_prompts_test.go` to pass and verify the new `baseBranch` parameter
