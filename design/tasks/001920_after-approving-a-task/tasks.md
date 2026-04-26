# Implementation Tasks

- [ ] In `frontend/src/pages/SpecTaskReviewPage.tsx`, change `handleApproved` to navigate to `project-task-detail` with `{ id: projectId, taskId }` instead of `project-specs` with `{ id: projectId, openTask: taskId }`. Update the inline comment to reflect the new intent.
- [ ] Build the frontend (`cd frontend && yarn build`) to confirm no type/compile errors.
- [ ] Manually verify in the inner Helix browser:
  - [ ] Open a SpecTask design review at `/orgs/.../projects/.../tasks/.../review/...`, click **Approve**, confirm the URL changes to `/orgs/.../projects/.../tasks/...` (task detail) and the desktop/chat is visible.
  - [ ] On a narrow viewport (<900px), confirm the same navigation lands with the `chat` tab active by default.
  - [ ] Click **Start Implementation** on the same page (when shown) and confirm the same redirect occurs.
  - [ ] Submit **Request Changes** and confirm there is NO redirect to the task detail page (existing close-only behaviour is preserved).
  - [ ] Open the workspace TabsView, open a review tab, click **Approve**, and confirm the review tab is replaced by the task tab in-place (no full-page navigation) — i.e. the workspace flow is unchanged.
  - [ ] Click browser **Back** from the redirected task detail page and confirm it returns to the review URL.
- [ ] Commit and push the change with a one-line message describing the user-visible behaviour.
