# Implementation Tasks

- [x] In `SpecTaskReviewPage.tsx`: add `handleApproved` function that navigates to `project-specs` with `openTask: taskId` param, and pass it as `onImplementationStarted` to `DesignReviewContent`
- [x] In `TabsView.tsx`: pass `onImplementationStarted` to `DesignReviewContent` that closes the review tab and opens/focuses the parent task tab in the same panel
- [x] Verify "request changes" flow is unaffected (only `onClose` is called, not `onImplementationStarted`)
