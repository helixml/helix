# Implementation Tasks

- [ ] Add `allTabsViewed` and `unviewedTabNames` props to `ReviewActionFooter` component (`frontend/src/components/spec-tasks/ReviewActionFooter.tsx`)
- [ ] Disable "Approve Design" button when `!allTabsViewed`, combining with existing `unresolvedCount > 0` check
- [ ] Add tooltip to disabled approve button showing which tabs haven't been viewed (use existing MUI Tooltip + span pattern from the "Start Implementation" button)
- [ ] In `DesignReviewContent.tsx`, compute `allTabsViewed` and `unviewedTabNames` from existing `viewedTabs` state and pass to `ReviewActionFooter`
- [ ] Test: load review page → verify approve button is disabled → click through all 3 tabs → verify button enables
- [ ] Run `cd frontend && yarn build` to verify no type/build errors
