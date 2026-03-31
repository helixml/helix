# Design: Fix Spec Review Auto-Navigation Loop

## Architecture

### Relevant Files

- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — contains the auto-open `useEffect` and the `handleReviewSpec` function, plus the sessionStorage helpers
- `frontend/src/pages/SpecTaskReviewPage.tsx` — the standalone spec review page (route: `/orgs/:org/projects/:id/tasks/:taskId/review/:reviewId`)

### Current Flow (Broken Path)

```
User opens spec review via direct URL / notification link
  → SpecTaskReviewPage mounts
  → (task is NOT marked in sessionStorage — only handleReviewSpec does that)
  → User clicks "Back"
  → SpecTaskDetailContent mounts
  → useEffect fires: task in spec_review, NOT in sessionStorage → auto-opens review again ❌
```

### Fixed Flow

```
User opens spec review (any path)
  → SpecTaskReviewPage mounts
  → useEffect marks task ID in shared sessionStorage key
  → User clicks "Back"
  → SpecTaskDetailContent mounts
  → useEffect fires: task in spec_review, IS in sessionStorage → no redirect ✓
```

## Key Decision

**Mark the task in sessionStorage from `SpecTaskReviewPage` on mount**, not only from `handleReviewSpec`.

This ensures the guard works regardless of how the user reached the review page. The sessionStorage key is already correct (`helix_auto_opened_spec_tasks`) — we just need to also write to it from the review page.

**Approach**: Extract the three sessionStorage helpers (`AUTO_OPENED_KEY`, `getAutoOpenedSpecTasks`, `addAutoOpenedSpecTask`) into a shared utility file, then import and call `addAutoOpenedSpecTask` in a `useEffect` inside `SpecTaskReviewPage`.

### Why Not Other Approaches

- **Navigation state flag** (`location.state.skipAutoOpen`): Only works for the `handleBack` click, not browser back button or other navigation paths.
- **Check referrer URL**: Fragile, not reliable across SPA navigation.
- **Disable the useEffect entirely**: Would break the intentional auto-open on first visit.

## Shared Utility Location

New file: `frontend/src/lib/specTaskAutoOpen.ts`

Exports:
```ts
export const AUTO_OPENED_KEY = "helix_auto_opened_spec_tasks";
export const getAutoOpenedSpecTasks = (): Set<string> => ...
export const addAutoOpenedSpecTask = (id: string): void => ...
```

Both `SpecTaskDetailContent.tsx` and `SpecTaskReviewPage.tsx` import from this file.

## Codebase Notes

- `SpecTaskDetailContent` polls task data every 2.3s — the useEffect dependency array includes `task?.id` and `task?.status`. The sessionStorage guard must be checked synchronously on every effect run.
- `handleReviewSpec` already calls `addAutoOpenedSpecTask` before the async API call — this remains unchanged.
- The `onOpenReview` callback path (workspace split-screen) also goes through `handleReviewSpec`, so it's already handled correctly and needs no changes.
