# Design: Auto-navigate After Spec Approval

## Architecture Overview

The spec review flow uses two entry points that render `DesignReviewContent`:

1. **`SpecTaskReviewPage`** (`src/pages/SpecTaskReviewPage.tsx`) — standalone full-page route
2. **`TabsView`** (`src/components/tasks/TabsView.tsx`) — the workspace tab panel

Both pass an `onClose` callback to `DesignReviewContent`. On approval, `DesignReviewContent` calls `onClose()` (and optionally `onImplementationStarted()` if provided).

## Key Files

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — approval logic at `handleSubmitReview()` (line ~852), calls `onClose()` after approval
- `frontend/src/pages/SpecTaskReviewPage.tsx` — `onClose` = `handleBack()` → navigates to `project-task-detail`
- `frontend/src/components/tasks/TabsView.tsx` (line ~1438) — `onClose` = `onTabClose(panel.id, activeTab.id)` → just closes the review tab

## Changes Required

### 1. `SpecTaskReviewPage.tsx` — Fix standalone page navigation

In `handleBack`, change destination from `project-task-detail` to `project-specs` so user lands in the workspace after approval.

```tsx
// Before
const handleBack = () => {
  account.orgNavigate('project-task-detail', { id: projectId, taskId })
}

// After: only change onClose for the approve path
// Option A: pass a separate onApproved prop to DesignReviewContent
// Option B: navigate to project-specs from handleBack when coming from approval

// Simplest: add onApproved prop or separate approval navigation in SpecTaskReviewPage
const handleApproved = () => {
  account.orgNavigate('project-specs', { id: projectId, openTask: taskId })
}
```

Pass `onImplementationStarted={handleApproved}` to `DesignReviewContent` (this callback already exists in the interface and is called after successful approval).

### 2. `TabsView.tsx` — Fix workspace tab navigation on approval

The `DesignReviewContent` in TabsView does not pass `onImplementationStarted`. Add it to switch focus to the task tab after approval closes the review tab.

```tsx
// In TabsView, where DesignReviewContent is rendered (line ~1438):
<DesignReviewContent
  key={`${panel.id}-${activeTab.id}`}
  specTaskId={activeTab.taskId}
  reviewId={activeTab.reviewId}
  onClose={() => onTabClose(panel.id, activeTab.id)}
  onImplementationStarted={() => {
    // Close the review tab first
    onTabClose(panel.id, activeTab.id)
    // Then open/switch to the parent task tab
    if (activeTab.taskId) {
      onOpenTask(panel.id, activeTab.taskId)  // use existing onOpenTask/handleAddTab mechanism
    }
  }}
  hideTitle={true}
/>
```

The `onOpenTask` would use the existing `handleAddTab` / `setRootNode` logic to either focus an existing task tab or open a new one.

## Decision: Use `onImplementationStarted` (not a new prop)

`DesignReviewContent` already has `onImplementationStarted?: () => void` which is called after successful approval but NOT after request-changes. This is the right hook to use — it preserves the existing `onClose` behavior for non-approval flows and adds targeted navigation only for the approval path.

## Codebase Patterns

- Navigation uses `account.orgNavigate(routeName, params)` — no direct router calls
- The `TabsView` uses `setRootNode` + `updateNodeInTree` to manage panels/tabs imperatively
- `onTabClose(panelId, tabId)` is the mechanism to close a tab
- Task tabs are opened via `handleAddTab(panelId, task)` or the `openTask` URL param mechanism
- `DesignReviewContent.onImplementationStarted` is already called at lines 868 and 912 in the approval branches
