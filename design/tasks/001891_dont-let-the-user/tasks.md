# Implementation Tasks

- [ ] Add `allTabsViewed` and `unviewedTabNames` props to `ReviewActionFooter` component (`frontend/src/components/spec-tasks/ReviewActionFooter.tsx`)
- [ ] Disable "Approve Design" button when `!allTabsViewed`, combining with existing `unresolvedCount > 0` check
- [ ] Add tooltip to disabled approve button showing which tabs haven't been viewed (use existing MUI Tooltip + span pattern from the "Start Implementation" button)
- [ ] In `DesignReviewContent.tsx`, compute `allTabsViewed` and `unviewedTabNames` from existing `viewedTabs` state and pass to `ReviewActionFooter`
- [ ] Add a `useRef<Map<DocumentType, string>>` to snapshot tab content when viewed. Update snapshot in `handleTabChange` and on initial mount for "requirements"
- [ ] Add a `useEffect` watching review data that compares current content to snapshots — if content changed for a viewed tab, remove it from `viewedTabs` (forces re-view)
- [ ] Add a visual indicator (colored dot badge) on tab labels for tabs with unread content changes
- [~] In `InlineCommentBubble.tsx`, replace `CloseIcon` with `CheckCircleIcon` (colored green) on the resolve button (line 130). Import `CheckCircleIcon` from `@mui/icons-material/CheckCircle`
- [ ] In `CommentLogSidebar.tsx`, replace `CloseIcon` with `CheckCircleIcon` (colored green) on the resolve button (line 78). `CheckCircleIcon` is already imported
- [ ] Test: load review page → verify approve button is disabled → click through all 3 tabs → verify button enables
- [ ] Test: verify resolve buttons on inline comments and comment log sidebar show green tick instead of X
- [ ] Run `cd frontend && yarn build` to verify no type/build errors
