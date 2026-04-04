# Use spec title as task name after spec push

## Summary

Once a spec is pushed, the task name automatically syncs to the first line of `requirements.md` (the spec title). This establishes a clear single source of truth for task naming: prompt-derived in backlog, spec-derived from review onwards.

## Changes

**Backend (`api/pkg/services/`):**
- Added `SpecTitleFromRequirements()` helper in `git_helpers.go` to extract the title from requirements.md (strips leading `#`, trims whitespace)
- Updated `createDesignReviewForPush()` in `git_http_server.go` to call the helper on every spec push and update the task name

**Frontend:**
- `SpecTaskDetailContent.tsx`: Made the prompt/description field read-only when task status is not in `backlog`, `queued_spec_generation`, or `spec_generation`
- `TaskCard.tsx`: Updated tooltip to show `original_prompt` (falling back to description then name)
- `SpecTaskDetailPage.tsx`: Updated breadcrumb tooltip to use original_prompt
- `SpecTaskReviewPage.tsx`: Added tooltip to task-name breadcrumb entry
