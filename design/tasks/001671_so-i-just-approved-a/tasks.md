# Implementation Tasks

- [ ] In `DesignReviewContent.handleSubmitReview`: add `return` after calling `onImplementationStarted()` so `onClose()` is not also called
- [ ] In `SpecTaskReviewPage.handleApproved`: write `taskId` into sessionStorage key `"helix_auto_opened_spec_tasks"` before navigating
- [ ] In `SpecTaskReviewPage.handleApproved`: change navigation destination from `project-specs` to `project-task-detail`
- [ ] Test: go directly to a spec review URL (not via auto-open), approve, confirm you land on task detail and stay there
- [ ] Build check: `cd frontend && yarn build`
