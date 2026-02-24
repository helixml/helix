# Implementation Tasks

- [ ] Update `PlanningPromptData` struct in `spec_task_prompts.go` to add `PrimaryRepoName` and `ReferenceRepos` fields
- [ ] Add "Repository Context" section to `planningPromptTemplate` that shows primary vs reference repos
- [ ] Update `BuildPlanningPrompt()` signature to accept `primaryRepoName string` and `repos []*types.GitRepository`
- [ ] Update `spec_task_orchestrator.go` `buildPlanningPrompt()` to identify primary repo from `project.DefaultRepoID` and pass to template
- [ ] Update `spec_driven_task_service.go` `StartSpecGeneration()` to fetch repos and pass to `BuildPlanningPrompt()`
- [ ] Move repository context section earlier in `approvalPromptTemplate` in `agent_instruction_service.go`
- [ ] Add reference repos list to implementation prompt (currently only shows primary)
- [ ] Update `BuildApprovalInstructionPrompt()` to accept full repos list for reference repos display
- [ ] Test with single repo project - verify it shows as PRIMARY with no reference section
- [ ] Test with multi-repo project - verify primary is distinguished from reference repos