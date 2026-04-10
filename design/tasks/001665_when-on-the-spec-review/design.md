# Design

## Change

Add the `tooltip` property to the task name breadcrumb in `SpecTaskReviewPage.tsx`, matching the existing pattern in `SpecTaskDetailPage.tsx`.

## Key Files

- `frontend/src/pages/SpecTaskReviewPage.tsx` — the only file that needs changing (line 92-96)
- `frontend/src/pages/SpecTaskDetailPage.tsx` — reference implementation (line 128-130)
- `frontend/src/components/system/Page.tsx` — breadcrumb renderer, already supports `tooltip` via MUI `<Tooltip>` (lines 192-201)

## What Already Exists

The breadcrumb system (`IPageBreadcrumb` type in `types.ts:1069-1076`) already has an optional `tooltip` field. The `Page.tsx` component already renders tooltips with `whiteSpace: 'pre-wrap'`, 500ms delay, and bottom-start placement. The `task` object is already fetched in `SpecTaskReviewPage` via `useSpecTask()` — `task.description` is available.

## Decision

One-line change: add `tooltip: task?.description || task?.name` to the existing breadcrumb object. No new components, hooks, or abstractions needed.
