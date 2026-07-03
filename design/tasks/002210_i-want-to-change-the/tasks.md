# Implementation Tasks: Add Open Questions Section to Spec Planning Prompt

- [ ] In `api/pkg/services/spec_task_prompts.go`, add a concise `## Open Questions (requirements.md)` instruction to `planningPromptTemplate`, placed after the "## Your Task Directory" list and before "## CRITICAL: Title Format"
- [ ] Ensure the instruction tells the agent to end `requirements.md` with an `## Open Questions` section listing genuine uncertainties, and to write "None" when there are none
- [ ] Add a test in `spec_task_prompts_test.go` asserting the rendered prompt contains the Open Questions instruction
- [ ] Run `go build ./api/pkg/services/` and `go test ./api/pkg/services/ -run TestBuildPlanningPrompt` to confirm the build and existing title-format test still pass
