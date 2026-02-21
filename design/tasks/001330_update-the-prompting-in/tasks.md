# Implementation Tasks

- [x] Update planning prompt template in `api/pkg/services/spec_task_prompts.go` to add optional visual testing section
- [x] Update implementation prompt template in `api/pkg/services/agent_instruction_service.go` to add screenshot/testing instructions
- [x] Add screenshot folder creation guidance to both prompts (screenshots/ subdirectory in task folder)
- [x] Add window focus workflow to prompts (list_windows → focus_window → save_screenshot)
- [ ] Test prompts render correctly by building Go code (`go build ./pkg/services/`)
- [ ] Push code changes to feature branch