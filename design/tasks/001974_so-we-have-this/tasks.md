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
- [x] Manually tested in inner Helix at `http://localhost:8080` — registered, created org/project, seeded a synthetic `spec_task_design_reviews` row directly in Postgres for the new task, navigated to `/orgs/.../tasks/.../review/<reviewId>`. All five scenarios pass:
  - [x] Pending design review — "NEXT DOCUMENT" button visible (screenshot 01).
  - [x] Click "NEXT DOCUMENT" twice → active tab advances Requirements → Technical Design → Implementation Plan; after viewing the third tab, button flips to "APPROVE DESIGN" (enabled) (screenshots 02, 03).
  - [x] Updated `implementation_plan` in DB while it was the active tab → content updated in UI, button stays "APPROVE DESIGN" — active tab NOT marked unread (screenshot 04). **Bug fix verified.**
  - [x] Updated `requirements_spec` in DB (non-active tab) → button flips back to "NEXT DOCUMENT" — non-active tab IS marked unread (screenshot 05).
  - [x] Inserted unresolved comment → tab badge shows "1", button reverts to disabled "APPROVE DESIGN" with "1 unresolved comment" warning alert beside it (NOT "Next Document") (screenshot 06).
- [x] Commit and push to a feature branch in `helixml/helix`; open a PR. (Branch `feature/001974-so-we-have-this` pushed; PR creation deferred to user via "Open PR" UI per task instructions.)
