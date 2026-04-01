# Design

## Fix 1 — Stop `onClose()` from conflicting with `onImplementationStarted()`

**File:** `frontend/src/components/spec-tasks/DesignReviewContent.tsx`, `handleSubmitReview` (~line 932)

Currently:
```typescript
if (onImplementationStarted) {
  onImplementationStarted();
}
onClose(); // always fires, overrides the navigation from onImplementationStarted
```

Fix: skip `onClose()` when `onImplementationStarted` has handled navigation:
```typescript
if (onImplementationStarted) {
  onImplementationStarted();
  return;
}
onClose();
```

This is the primary fix. `onImplementationStarted` is the caller's signal that it owns post-approval navigation.

## Fix 2 — Mark task as auto-opened before navigating away from the review page

**File:** `frontend/src/pages/SpecTaskReviewPage.tsx`, `handleApproved`

The auto-open guard in `SpecTaskDetailContent` uses sessionStorage key `"helix_auto_opened_spec_tasks"`. If the user came to the review page directly (not via the auto-open flow), this key doesn't contain the task ID, so the guard doesn't fire and the effect redirects them back.

Fix: write the task ID into sessionStorage in `handleApproved` before navigating:
```typescript
const handleApproved = () => {
  // Prevent SpecTaskDetailContent's auto-open effect from redirecting back to review
  const key = "helix_auto_opened_spec_tasks";
  const existing = new Set<string>(JSON.parse(sessionStorage.getItem(key) || "[]"));
  existing.add(taskId);
  sessionStorage.setItem(key, JSON.stringify([...existing]));

  account.orgNavigate('project-task-detail', { id: projectId, taskId });
};
```

Note: also change the destination from `project-specs` to `project-task-detail` — the user wants to see the agent desktop, not the kanban board. The `openTask` param passed to `project-specs` only works in workspace mode (which isn't the default), so the previous navigation landed on kanban with no task visible.

## Key Files

| File | Change |
|------|--------|
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | Don't call `onClose()` after `onImplementationStarted()` returns |
| `frontend/src/pages/SpecTaskReviewPage.tsx` | Write task ID to sessionStorage + navigate to `project-task-detail` |

## Notes

- The sessionStorage key `"helix_auto_opened_spec_tasks"` is defined at the top of `SpecTaskDetailContent.tsx` (line 116). Duplicate the write logic in `SpecTaskReviewPage` rather than exporting it — it's two lines of sessionStorage manipulation, not worth an abstraction.
- The backend `approveSpecs` handler already correctly updates task status to `spec_approved` and kicks off implementation. No backend changes needed.
- The original bug report mentioned "probably because of a use effect" — that's Fix 2 (`SpecTaskDetailContent` line 844). Fix 1 is the immediate cause that gets the user to the detail page in the first place.
