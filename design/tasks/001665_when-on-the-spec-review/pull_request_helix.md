# Add tooltip to task breadcrumb on spec review page

## Summary
The task name breadcrumb on the spec review page was missing the hover tooltip that already exists on the spec task detail page. Added `tooltip: task?.description || task?.name` to show the full task description on hover.

## Changes
- Added `tooltip` property to the task name breadcrumb in `SpecTaskReviewPage.tsx`
