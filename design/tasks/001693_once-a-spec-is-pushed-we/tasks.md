# Implementation Tasks

- [x] Add `specTitleFromRequirements(content string) string` helper in `git_http_server.go` (strip leading `#`, trim whitespace, return first non-empty line)
- [x] In `processDesignDocsForBranch()`, call the helper on `requirements.md` content and update the task `Name` field on every push (not only on the initial `spec_review` transition); skip if title is empty
- [x] In `SpecTaskDetailContent.tsx`, make the description/prompt `TextField` read-only when task status is not in `["backlog", "queued_spec_generation", "spec_generation"]`
- [~] In `TaskCard.tsx` (kanban card), update the name tooltip to use `task.original_prompt || task.description || task.name`
- [ ] In `SpecTaskDetailPage.tsx`, update the breadcrumb tooltip to use `task?.original_prompt || task?.description || task?.name`
- [ ] In `SpecTaskReviewPage.tsx`, add a `tooltip` field to the task-name breadcrumb entry using `task?.original_prompt || task?.description || task?.name`
