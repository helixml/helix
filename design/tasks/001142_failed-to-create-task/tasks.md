# Implementation Tasks

- [ ] Remove the "active task on branch" validation block in `api/pkg/services/spec_driven_task_service.go` (lines ~152-170) that prevents creating tasks when `BranchModeExisting` is used and an active task already exists on the branch