# Implementation Tasks

- [ ] In `git_http_server.go` → `processDesignDocsForBranch()`, extract the title from `requirements.md` content using a `specTitleFromRequirements()` helper (strip leading `#`, trim whitespace, take first non-empty line)
- [ ] Update the task's `Name` field with the extracted title when transitioning to `spec_review` (skip update if title is empty)
- [ ] In `SpecTaskDetailContent.tsx`, make the description/prompt `TextField` read-only when task status is not in `["backlog", "queued_spec_generation", "spec_generation"]`
