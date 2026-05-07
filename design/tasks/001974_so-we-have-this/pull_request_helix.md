# Fix active-tab unread bug and add Next Document button to design review

## Summary

Two related changes to the design review screen (`DesignReviewContent.tsx` + `ReviewActionFooter.tsx`):

1. **Bug fix** — when content updates arrive for the tab the user is currently viewing, the tab no longer gets a red "unread" dot. The user is already looking at the new content; flagging it is wrong. Other (non-active) tabs continue to be marked unread when their content changes.
2. **UX** — when there are unread tabs, the "Approve Design" button is replaced with an enabled **"Next Document"** button that jumps to the next unread tab. People kept missing the disabled-button tooltip; now the button itself drives the workflow forward — three clicks in the worst case to view all tabs.

## Changes

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx`
  - Content-invalidation `useEffect`: skip the active tab and refresh its snapshot in place rather than removing it from `viewedTabs`. Added `activeTab` to the dependency array.
  - Added `handleNextDocument` — picks the next unread tab in canonical order (Requirements → Technical Design → Implementation Plan, wrapping) and routes through the existing `handleTabChange`.
  - Removed the now-unused `unviewedTabNames` computation.
- `frontend/src/components/spec-tasks/ReviewActionFooter.tsx`
  - New props: `hasNextDocument?: boolean`, `onNextDocument?: () => void`.
  - When `hasNextDocument && unresolvedCount === 0`, render a primary "Next Document" button. Otherwise render the existing "Approve Design" button (still disabled when `unresolvedCount > 0 || !allTabsViewed`).
  - Removed the `unviewedTabNames` prop and the now-dead disabled-button tooltip text.

## Notes

- Why does `unresolvedCount > 0` block "Next Document"? Unresolved comments mean the user can't approve regardless of read state. Showing "Next Document" while the end button can't be clicked would be misleading. The unresolved-comments alert (rendered to the left) already explains the block.
- Pure frontend change — no backend, types, or API client changes.
- TypeScript (`yarn tsc`) and full Vite build (`yarn build`) pass cleanly.

## Test Plan

- [ ] Open a spec task with a pending design review — "Next Document" button visible, red dots on unviewed tabs.
- [ ] Click "Next Document" repeatedly — active tab advances, red dot clears, button flips to "Approve Design" (enabled) once all tabs viewed.
- [ ] Trigger a content change while viewing a tab — that tab does NOT get a red dot.
- [ ] Trigger a content change to a non-active tab — it DOES get a red dot.
- [ ] Add an unresolved comment — button reverts to disabled "Approve Design" with the existing unresolved-comments alert (not "Next Document").
