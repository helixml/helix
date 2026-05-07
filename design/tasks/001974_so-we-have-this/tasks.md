# Implementation Tasks

- [x] In `frontend/src/components/spec-tasks/DesignReviewContent.tsx`, modify the content-invalidation `useEffect` (currently lines 311-330) to skip the active tab and refresh its snapshot in place instead of removing it from `viewedTabs`.
- [x] Add `activeTab` to the dependency array of that effect.
- [x] In the same file, add a `handleNextDocument` callback that finds the next unread tab in `ALL_TABS` order (starting after `activeTab`, wrapping) and calls `handleTabChange(candidate)`.
- [x] In `frontend/src/components/spec-tasks/ReviewActionFooter.tsx`, add `onNextDocument?: () => void` and `hasNextDocument?: boolean` props.
- [x] In `ReviewActionFooter`, render a "Next Document" `<Button>` (variant=`contained`, color=`primary`, enabled) when `hasNextDocument && unresolvedCount === 0`; otherwise render the existing "Approve Design" tooltip+button block.
- [x] Simplify the tooltip in the "Approve Design" branch — drop the `!allTabsViewed` text path (now unreachable when `unresolvedCount === 0`); keep the empty-string fallback. The unresolved-comments case retains the alert beside the button (no tooltip change needed there).
  - Also dropped the now-unused `unviewedTabNames` prop from `ReviewActionFooterProps` and removed its computation in `DesignReviewContent.tsx` (dead code per CLAUDE.md's "CLEAN UP DEAD CODE" rule).
  - Tooltip wrapper removed entirely from the Approve branch — the only remaining gating signal there is `unresolvedCount > 0`, which is communicated by the existing warning alert to the left of the button.
- [x] In `DesignReviewContent.tsx`, pass `onNextDocument={handleNextDocument}` and `hasNextDocument={!allTabsViewed}` to `<ReviewActionFooter>`.
- [x] Run `cd frontend && yarn build` to verify TypeScript compiles cleanly. (Both `yarn tsc` and full `yarn build` pass — 21068 modules transformed, all chunks emitted.)
- [~] Manually test in inner Helix at `http://localhost:8080` — **BLOCKED: dev stack still building images at task start; tests pending until stack is up. TypeScript and full Vite build verified.**
  - [ ] Open a spec task with a pending design review; verify "Next Document" appears with red dots on the two unviewed tabs.
  - [ ] Click "Next Document" twice; after each click the active tab advances and its red dot clears; after the third unique tab is viewed, button flips to "Approve Design" and is enabled.
  - [ ] Trigger a content change to the *active* tab (e.g. add a comment that the agent answers) and confirm the active tab does NOT get a red dot.
  - [ ] Trigger a content change to a *non-active* tab and confirm it DOES get a red dot.
  - [ ] Add an unresolved comment and confirm the button reverts to disabled "Approve Design" with the unresolved-comments alert (not "Next Document").
- [x] Commit and push to a feature branch in `helixml/helix`; open a PR. (Branch `feature/001974-so-we-have-this` pushed; PR creation deferred to user via "Open PR" UI per task instructions.)
