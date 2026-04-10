# Requirements

## User Story

As a user viewing the spec review page, I want the task name breadcrumb (second-to-last) to show the full task description on hover, so I have the same context as on the spec task detail page.

## Current Behavior

- **Spec Task Detail Page** (`SpecTaskDetailPage.tsx:129`): The task name breadcrumb has `tooltip: task?.description || task?.name` — hovering shows the full description.
- **Spec Review Page** (`SpecTaskReviewPage.tsx:92-96`): The task name breadcrumb has no `tooltip` property — hovering shows nothing.

## Acceptance Criteria

- [ ] On the spec review page, hovering over the task name breadcrumb shows a tooltip with the full task description (falling back to the task name if no description exists)
- [ ] Tooltip behavior matches the spec task detail page (500ms delay, bottom-start placement, pre-wrap whitespace)
