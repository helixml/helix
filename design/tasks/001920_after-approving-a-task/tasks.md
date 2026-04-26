# Implementation Tasks

- [ ] Check if branch `feature/001661-occasionally-when-i` (commit `776bc2ad4`) has already been merged to `main`. Run: `git log origin/main -- frontend/src/lib/specTaskAutoOpen.ts`. If the file exists on main, skip the "Extract sessionStorage helpers" and "Mark task as auto-opened on review page mount" steps below — they're already done.
- [ ] **Extract sessionStorage helpers** to a new file `frontend/src/lib/specTaskAutoOpen.ts` exporting `AUTO_OPENED_KEY`, `getAutoOpenedSpecTasks()`, and `addAutoOpenedSpecTask(id)` (matches commit `776bc2ad4` exactly).
- [ ] **Update `frontend/src/components/tasks/SpecTaskDetailContent.tsx`** to import `getAutoOpenedSpecTasks` and `addAutoOpenedSpecTask` from `../../lib/specTaskAutoOpen` and delete the inline copies (lines 119–129).
- [ ] **Mark task as auto-opened on review page mount**: in `frontend/src/pages/SpecTaskReviewPage.tsx`, import `addAutoOpenedSpecTask` and add a `useEffect(() => { if (taskId) addAutoOpenedSpecTask(taskId) }, [taskId])`.
- [ ] **Defence-in-depth check**: in the auto-open `useEffect` in `SpecTaskDetailContent.tsx` (currently lines 921–932), add `!task?.spec_approved_at` to the condition and add `task?.spec_approved_at` to the dependency array.
- [ ] **Change navigation target**: in `frontend/src/pages/SpecTaskReviewPage.tsx`, change `handleApproved` to call `account.orgNavigate('project-task-detail', { id: projectId, taskId })` instead of `account.orgNavigate('project-specs', { id: projectId, openTask: taskId })`. Update the inline comment to reflect the new intent.
- [ ] Build the frontend (`cd frontend && yarn build`) to confirm no type/compile errors.
- [ ] Manually verify in the inner Helix browser:
  - [ ] Open a SpecTask design review page directly via URL (`/orgs/.../projects/.../tasks/.../review/...`), click **Approve**, confirm URL changes to `/orgs/.../projects/.../tasks/...` and the page **stays** on the desktop/chat view (does NOT bounce back to `/review/...`).
  - [ ] On a narrow viewport (<900px), repeat the approval and confirm the destination shows the `chat` tab by default.
  - [ ] Click **Start Implementation** on the review page (when shown) and confirm the same redirect + non-bounce.
  - [ ] Submit **Request Changes** and confirm there is NO redirect to the task detail page (existing close-only behaviour).
  - [ ] Open the workspace TabsView, open a review tab, click **Approve**, and confirm the review tab is replaced by the task tab in-place (no full-page navigation) — workspace flow unchanged.
  - [ ] Click browser **Back** from the redirected task detail page and confirm it returns to the review URL **and** does not auto-bounce away from there.
  - [ ] Open a brand-new spec task that is in `spec_review` status with design docs pushed, in a fresh browser tab (cleared sessionStorage). Navigate directly to the task detail page (NOT the review page). Confirm the auto-open useEffect still fires and redirects to the review page — i.e. the original "auto-open on first visit" behaviour for unapproved tasks is preserved.
- [ ] Commit and push with a message describing both the navigation change and the auto-open root-cause fix.
