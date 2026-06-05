# Requirements: Synchronous Project Delete with Visible Spinner

## Problem

When a user deletes a project from `ProjectSettings`, the confirmation dialog closes
immediately after clicking the final **Delete Project** button and the user is
navigated back to the projects list — where the just-deleted project is still
visible. Clicking it and attempting to delete again yields "project not found"
because the backend already deleted it on the first request.

The delete *feels* asynchronous because the projects list shows stale data after
the dialog closes. In reality the backend `DELETE /api/v1/projects/{id}` handler
is synchronous, but the React Query cache for the projects list is **not**
invalidated correctly (key mismatch — see `design.md`), so the UI keeps showing
the old list.

## User Story

> As a user, when I delete a project I want the confirmation dialog to stay open
> with a clear spinner until the delete has fully completed *and* the project
> list has been refreshed, so that when I return to the list the project is
> gone and I'm not tempted to click it again.

## Acceptance Criteria

1. **Spinner visible for the full operation.** After clicking the final
   **Delete Project** button, a spinner is shown on that button (and the button
   is disabled) until the backend has responded AND the projects list cache has
   been refetched. "Deleting…" label is shown.
2. **Dialog cannot be dismissed mid-delete.** While the delete is in flight,
   clicking outside the dialog, pressing Escape, and the **Cancel** button all
   do nothing. The user cannot leave the dialog in a partial state.
3. **No stale project after delete.** When the dialog finally closes and the
   user lands on the projects list (or any other page that uses
   `useListProjects`), the deleted project is no longer present. The user
   never sees a "ghost" of the deleted project.
4. **No double-delete 404.** A user following the normal click-through flow
   should never be able to fire a second `DELETE` against the same project
   id and receive a 404 because the first delete already succeeded.
5. **Toast feedback** is shown once at the end: `"Project deleted successfully"`
   on success, or `"Failed to delete project"` on failure. Failure leaves the
   dialog open so the user can retry or cancel.
6. **No regression** for the existing typing-the-name-to-confirm guard — the
   Delete button stays disabled until the typed name matches exactly.

## Out of Scope

- Hard-delete of orphaned `spec_tasks`, `sandboxes`, `prompt_history_entries`
  rows. The backend currently leaves these orphaned after a project soft-delete;
  cleaning them up is a separate concern and not what the user reported.
- Changing the backend handler's behaviour (it is already synchronous).
- Restructuring the dialog component or moving the delete entry-point.
