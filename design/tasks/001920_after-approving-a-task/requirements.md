# Requirements

## Problem

After a user approves a spec design from the standalone Spec Review page (`/orgs/:org_id/projects/:id/tasks/:taskId/review/:reviewId`), the app does not land them in the chat/desktop where the agent is now implementing. Two distinct mechanisms conspire here:

1. **Explicit redirect**: `SpecTaskReviewPage.handleApproved` navigates to `project-specs?openTask=...` (the project Kanban), not to the task's chat/desktop.
2. **Auto-open useEffect** (the root cause the user is asking about): `SpecTaskDetailContent.tsx:921` auto-redirects from the task detail page to the spec review page whenever it sees `task.status === spec_review|spec_revision` AND `task.design_docs_pushed_at` set AND the task ID is not yet in the `helix_auto_opened_spec_tasks` sessionStorage set. Because:
   - The sessionStorage dedupe is only written by `SpecTaskDetailContent.handleReviewSpec` (line 889), it is **not** written when the user reaches the review page via deep link, notification, breadcrumb, or our post-approval redirect.
   - React Query invalidates the task on approve, but the refetch is async — for a brief window the cached `task.status` is still `spec_review`. So even with our fix to (1), landing on the task detail page would immediately re-redirect to the spec.

This auto-open useEffect has been "problematic for some time in jumping you to the spec in other cases" — there is already an unmerged fix at commit `776bc2ad4` on branch `feature/001661-occasionally-when-i` that extracts the sessionStorage helpers into a shared lib and writes the dedupe entry on `SpecTaskReviewPage` mount. Our task supersedes / absorbs that fix because changing only the navigation target without addressing the useEffect would still leave the bug.

## User Story

As a user who has just approved a task's design,
I want to be taken straight to the chat/desktop view for that task and stay there,
So I can immediately watch the agent implement and chat with it without being bounced back to the spec.

## Acceptance Criteria

- **AC1**: When a user clicks **Approve** on the standalone Spec Review page (`SpecTaskReviewPage`) and the request succeeds, the app MUST land on the SpecTask detail page for that task (`project-task-detail`), NOT on `project-specs`.
- **AC2**: After landing on the task detail page from approval, the auto-open spec-review useEffect in `SpecTaskDetailContent` MUST NOT bounce the user back to the review page — even if the cached `task.status` is briefly stale at `spec_review` while React Query refetches.
- **AC3**: The same destination + non-bounce behaviour applies when the user clicks **Start Implementation** on the review page (the second site that fires `onImplementationStarted` in `DesignReviewContent.tsx`).
- **AC4**: Submitting **Request Changes** keeps the existing behaviour (close the review, no automatic navigation to the task detail).
- **AC5**: The in-workspace flow (review opened as a tab inside `TabsView`) MUST be unchanged. It already replaces the review tab with the task tab, which renders the desktop+chat — that behaviour is correct and must be preserved.
- **AC6**: The auto-open behaviour for unapproved tasks is preserved: a user opening a task that is genuinely waiting on review (status `spec_review`/`spec_revision`, design docs pushed, not yet visited this SPA session) still gets auto-redirected to the review page on first visit.
- **AC7**: Browser **Back** from the redirected task detail page returns the user to the spec review URL they came from (no `replace` redirect). The auto-open MUST NOT then re-bounce them off the review page either, because they actively chose to go back.
- **AC8**: On a wide viewport, the destination task detail page lands on the `desktop` view (chat panel visible alongside). On mobile, the existing default of `chat` view applies.

## Out of Scope

- No backend changes. The approval API itself is unchanged.
- No change to the implementation-approval / merge flow (`approveImplementation` button).
- No change to the standalone `TeamDesktopPage` (Human Desktop / exploratory session) navigation.
- We do not redesign the auto-open semantics beyond the minimal fix needed to stop the bounce — e.g. we are not removing the auto-open feature itself, since it is the desired behaviour for the first visit to a task awaiting review.
