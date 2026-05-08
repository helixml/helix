# Implementation Tasks

- [x] Add `syncViewWithUrl?: boolean` prop to `SpecTaskDetailContent` (default `true` for backward compat)
- [x] Guard `router.mergeParams({ view: newView })` calls in `SpecTaskDetailContent` with `syncViewWithUrl`
- [x] Guard the `useEffect` that watches `router.params.view` with `syncViewWithUrl`
- [x] Find where `SpecTaskDetailContent` is rendered inside `TabsView.tsx` and pass `syncViewWithUrl={false}`
- [x] Verify: changing tab in one split-screen panel does not affect the other panel's tab (verified by code analysis + tsc; live browser test blocked — stack not up)
- [x] Verify: single-panel URL-param tab init (`?view=details`) still works correctly (verified by code analysis — default `syncViewWithUrl=true` in `SpecTaskDetailPage` preserves all original code paths)
