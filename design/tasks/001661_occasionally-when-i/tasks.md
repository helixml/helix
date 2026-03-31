# Implementation Tasks

- [x] Create `frontend/src/lib/specTaskAutoOpen.ts` with the three sessionStorage helpers extracted from `SpecTaskDetailContent.tsx` (`AUTO_OPENED_KEY`, `getAutoOpenedSpecTasks`, `addAutoOpenedSpecTask`)
- [x] Update `SpecTaskDetailContent.tsx` to import these helpers from the new shared file instead of defining them inline
- [x] In `SpecTaskReviewPage.tsx`, import `addAutoOpenedSpecTask` and add a `useEffect` that calls it with `taskId` when the component mounts (dependency: `[taskId]`)
- [x] Verify the fix: navigate to a spec review page directly by URL, then click Back — confirm you stay on the task detail page
- [x] Run `cd frontend && yarn build` to confirm no TypeScript errors (tsc --noEmit passes; dist write fails due to pre-existing permissions issue unrelated to this change)
