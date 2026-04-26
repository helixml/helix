# Requirements

## Problem

After a user approves a spec design from the standalone Spec Review page (`/orgs/:org_id/projects/:id/tasks/:taskId/review/:reviewId`), the app navigates them back to the project Kanban / spec list (`project-specs?openTask=...`). At that point the agent is already running in the chat/desktop view; the spec page is no longer where the action is, so this extra hop is wasted clicks.

## User Story

As a user who has just approved a task's design,
I want to be taken straight to the chat/desktop view for that task,
So I can immediately watch the agent implement the work and chat with it without an extra navigation step.

## Acceptance Criteria

- **AC1**: When a user clicks **Approve** on the standalone Spec Review page (`SpecTaskReviewPage`) and the request succeeds, the app MUST navigate to the SpecTask detail page for that task (`project-task-detail`), NOT to `project-specs`.
- **AC2**: On a wide viewport, the destination page SHOULD land on the `desktop` view by default (chat panel visible alongside). On mobile, the existing default of `chat` view applies.
- **AC3**: The navigation MUST happen only on the `approve` decision path. Submitting **Request Changes** must keep the existing behaviour (close the review, no automatic navigation to the task detail).
- **AC4**: The same redirect behaviour applies when the user clicks **Start Implementation** on the review page (the second site that fires `onImplementationStarted` in `DesignReviewContent.tsx`).
- **AC5**: The in-workspace flow (review opened as a tab inside `TabsView`) MUST be unchanged. It already replaces the review tab with the task tab, which renders the desktop+chat — that behaviour is correct and must be preserved.
- **AC6**: Browser **Back** from the task detail page should return the user to the spec review URL they came from (router default behaviour — do not push a `replace` redirect).

## Out of Scope

- No backend changes. The approval API itself is unchanged.
- No change to the implementation-approval / merge flow (`approveImplementation` button).
- No change to the standalone `TeamDesktopPage` (Human Desktop / exploratory session) navigation.
