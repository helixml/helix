# Implementation Tasks

- [~] In `frontend/src/components/spec-tasks/DesignReviewContent.tsx`, modify the content-invalidation `useEffect` (currently lines 311-330) to skip the active tab and refresh its snapshot in place instead of removing it from `viewedTabs`.
- [~] Add `activeTab` to the dependency array of that effect.
- [~] In the same file, add a `handleNextDocument` callback that finds the next unread tab in `ALL_TABS` order (starting after `activeTab`, wrapping) and calls `handleTabChange(candidate)`.
- [~] In `frontend/src/components/spec-tasks/ReviewActionFooter.tsx`, add `onNextDocument?: () => void` and `hasNextDocument?: boolean` props.
- [~] In `ReviewActionFooter`, render a "Next Document" `<Button>` (variant=`contained`, color=`primary`, enabled) when `hasNextDocument && unresolvedCount === 0`; otherwise render the existing "Approve Design" tooltip+button block.
- [~] Simplify the tooltip in the "Approve Design" branch — drop the `!allTabsViewed` text path (now unreachable when `unresolvedCount === 0`); keep the empty-string fallback. The unresolved-comments case retains the alert beside the button (no tooltip change needed there).
- [~] In `DesignReviewContent.tsx`, pass `onNextDocument={handleNextDocument}` and `hasNextDocument={!allTabsViewed}` to `<ReviewActionFooter>`.
- [ ] Run `cd frontend && yarn build` to verify TypeScript compiles cleanly.
- [ ] Manually test in inner Helix at `http://localhost:8080`:
  - [ ] Open a spec task with a pending design review; verify "Next Document" appears with red dots on the two unviewed tabs.
  - [ ] Click "Next Document" twice; after each click the active tab advances and its red dot clears; after the third unique tab is viewed, button flips to "Approve Design" and is enabled.
  - [ ] Trigger a content change to the *active* tab (e.g. add a comment that the agent answers) and confirm the active tab does NOT get a red dot.
  - [ ] Trigger a content change to a *non-active* tab and confirm it DOES get a red dot.
  - [ ] Add an unresolved comment and confirm the button reverts to disabled "Approve Design" with the unresolved-comments alert (not "Next Document").
- [ ] Commit and push to a feature branch in `helixml/helix`; open a PR.
