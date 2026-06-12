# Requirements: Spinner on Approve Button in Review Submit Dialog

## Background

When a user approves a design review in the spec-task review flow, a dialog
(`ReviewSubmitDialog`) prompts them for an optional comment and shows an
"Approve" button. Today, clicking that button disables it but provides no
visual feedback. The underlying API call (`POST
/api/v1/spec-tasks/{id}/design-reviews/{review_id}/submit`) can take several
seconds — it validates GitHub OAuth, marks the review as approved, and
kicks off implementation. Users see nothing happening and may click again
or assume the app is broken.

## User Story

As a user approving a design review with an optional comment,
I want to see a loading spinner on the Approve button after I click it,
so that I know my approval is in flight and I don't double-click or assume
the app is stuck.

## Acceptance Criteria

1. **Spinner appears on submission.** When the user clicks the primary
   confirmation button in `ReviewSubmitDialog` (whether "Approve" or
   "Submit Feedback"), a `CircularProgress` indicator appears inside the
   button while the mutation is in flight (`isSubmitting === true`).
2. **Button text reflects state.** While submitting, the button label
   changes from "Approve" to "Approving…" (and from "Submit Feedback" to
   "Submitting…"). When `isSubmitting` returns to `false`, the original
   label is restored.
3. **Button stays disabled during submission.** Existing
   `disabled={isSubmitting}` behaviour is preserved — the user cannot
   trigger a second submission while one is in flight.
4. **Cancel remains usable.** The "Cancel" button is not affected by
   the new spinner. (No new disabled state on Cancel.)
5. **Matches existing pattern.** The spinner uses the same
   `<CircularProgress size={16} color="inherit" />` pattern already used
   throughout the codebase (e.g. `SpecTaskActionButtons.tsx`,
   `AddMcpSkillDialog.tsx`, `DuplicateDialog.tsx`), placed via the MUI
   `startIcon` prop.
6. **No regression for the negative path.** If the submission fails, the
   spinner disappears and the original button label/state is restored so
   the user can retry. This already happens for free via the
   `isSubmitting` prop driven by React Query.

## Out of Scope

- No backend changes. The mutation, endpoint, and OAuth validation stay
  identical.
- No change to the "Approve Design" button in `ReviewActionFooter.tsx`
  that *opens* this dialog — that button is not the focus of the user's
  complaint and already has appropriate guarded state.
- No change to the "Start Implementation" button (separate flow,
  `v1SpecTasksApproveImplementationCreate`).
- No change to the dialog's text field, layout, or comment-handling logic.
