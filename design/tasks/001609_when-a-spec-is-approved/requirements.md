# Requirements: Auto-navigate After Spec Approval

## User Story

As a developer reviewing a spec, when I approve the spec from the spec review view, I want to be automatically navigated back to the chat/desktop workspace view so I can monitor the implementation progress without extra clicks.

## Current Behavior

When a spec is approved from:
1. **Standalone review page** (`SpecTaskReviewPage`): `onClose()` calls `handleBack()` which navigates to `project-task-detail` (task detail page), not the workspace/chat view.
2. **Tabs workspace** (`TabsView`): `onClose()` calls `onTabClose(panel.id, activeTab.id)` which closes the review tab but just reveals the adjacent tab — there's no active navigation to the task's chat tab or desktop tab.

## Desired Behavior

After approving a spec:
- **In the tabs workspace view**: close the review tab AND switch the active tab to the associated task's detail tab (which shows the session/chat where the agent will be working).
- **In the standalone review page**: navigate to `project-specs` (workspace) instead of `project-task-detail`, so the user lands in the workspace and can see the task's chat/agent activity.

## Acceptance Criteria

- [ ] Approving a spec in `SpecTaskReviewPage` navigates to the `project-specs` route (workspace view) instead of `project-task-detail`.
- [ ] Approving a spec in the `TabsView` workspace closes the review tab and switches focus to the corresponding task's tab (not just "the next tab").
- [ ] Requesting changes (non-approval) retains existing navigation behavior (navigates back, does not switch to task tab).
- [ ] If no task tab is open in the workspace when a review is approved, the task tab is opened automatically.
