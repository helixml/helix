# Implementation Tasks

- [ ] Add `syncViewWithUrl?: boolean` prop to `SpecTaskDetailContent` (default `true` for backward compat)
- [ ] Guard `router.mergeParams({ view: newView })` calls in `SpecTaskDetailContent` with `syncViewWithUrl`
- [ ] Guard the `useEffect` that watches `router.params.view` with `syncViewWithUrl`
- [ ] Find where `SpecTaskDetailContent` is rendered inside `TabsView.tsx` and pass `syncViewWithUrl={false}`
- [ ] Verify: changing tab in one split-screen panel does not affect the other panel's tab
- [ ] Verify: single-panel URL-param tab init (`?view=details`) still works correctly
