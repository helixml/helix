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

All five scenarios verified manually in the inner Helix browser (screenshots in `helix-specs` task directory):

- [x] Pending design review with two unviewed tabs → "NEXT DOCUMENT" button visible.
- [x] Click "NEXT DOCUMENT" twice → active tab advances Requirements → Technical Design → Implementation Plan; after the third tab is viewed, button flips to "APPROVE DESIGN" (enabled).
- [x] Updated active tab's content via DB → content refreshes in UI, button stays "APPROVE DESIGN" (active tab NOT marked unread). **Bug fix verified.**
- [x] Updated a non-active tab's content via DB → button flips back to "NEXT DOCUMENT" (non-active tab IS marked unread).
- [x] Inserted unresolved comment via DB → tab badge shows "1", button is disabled "APPROVE DESIGN" with "1 unresolved comment" warning Alert (NOT "Next Document").

## Screenshots

![Initial: Next Document button with two unviewed tabs](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001974_so-we-have-this/screenshots/01-next-document-on-requirements.png)
![After first Next click: Technical Design active](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001974_so-we-have-this/screenshots/02-after-first-next-on-technical-design.png)
![After second Next click: Approve Design enabled](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001974_so-we-have-this/screenshots/03-approve-design-after-all-tabs-viewed.png)
![Bug fix: active tab content changed, button stays Approve](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001974_so-we-have-this/screenshots/04-active-tab-content-changed-still-approve.png)
![Non-active tab content changed: button flips back to Next](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001974_so-we-have-this/screenshots/05-non-active-tab-changed-button-flips-to-next.png)
![Unresolved comment: disabled Approve, NOT Next Document](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001974_so-we-have-this/screenshots/06-unresolved-comment-disables-approve-not-next.png)
