# Make split-screen spec task panels own their view tab independently

## Summary

In the split-screen workspace (`TabsView`), changing the view tab (chat / desktop / changes / details) on one visible spec task changed it on **all** visible spec tasks. Each panel now owns its tab state independently.

## Root cause

`SpecTaskDetailContent` synced `currentView` bidirectionally with the URL query param `view`:

- `getInitialView()` read `router.params.view` on mount.
- A `useEffect` watched `router.params.view` and pushed it into local state.
- `handleViewChange` (and a fallback effect that auto-switches on session state) called `router.mergeParams({ view })` on every change.

When two instances were mounted in split-screen, a tab change in one updated the URL, which the other's URL-watching effect immediately mirrored — so all panels stayed in lockstep.

## Changes

- `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
  - Add `syncViewWithUrl?: boolean` prop (default `true` for backward compatibility).
  - Skip URL read in `getInitialView` when false.
  - Early-return from URL-watching `useEffect` when false.
  - Guard all three `router.mergeParams({ view })` write sites with `syncViewWithUrl`.
- `frontend/src/components/tasks/TabsView.tsx`
  - Pass `syncViewWithUrl={false}` to `SpecTaskDetailContent` rendered inside a panel.

`SpecTaskDetailPage` (the single-task page) keeps the default `syncViewWithUrl=true`, so direct navigation to `?view=details` still works.

## Test plan

- [x] `npx tsc --noEmit` passes.
- [x] Vite build transforms cleanly (final `dist/` write blocked by environment-only permissions issue).
- [ ] Manual: open two spec tasks side-by-side in split-screen; switch one to "details", confirm the other stays put.
- [ ] Manual: navigate to a single spec task with `?view=details` in the URL — view initialises to details.
