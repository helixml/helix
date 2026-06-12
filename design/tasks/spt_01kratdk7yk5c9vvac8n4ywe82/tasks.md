# Implementation Tasks: Spinner on Approve Button in Review Submit Dialog

- [x] Add `CircularProgress` to the MUI imports in `frontend/src/components/spec-tasks/ReviewSubmitDialog.tsx`.
- [x] Update the primary confirmation `<Button>` to render a `startIcon` of `<CircularProgress size={16} color="inherit" />` when `isSubmitting` is true (else `undefined`).
- [x] Update the button label so that while `isSubmitting` is true it reads "Approving…" for the approve decision and "Submitting…" for the request-changes decision; restore "Approve" / "Submit Feedback" otherwise.
- [~] Run `cd frontend && yarn build` and confirm the build passes.
- [ ] Manually verify in the inner Helix dev environment (`http://localhost:8080`) that opening the review submit dialog, clicking "Approve", shows a spinner with "Approving…" label until the dialog closes.
- [ ] Manually verify the "Request Changes" path shows the spinner with "Submitting…".
- [ ] Commit with a conventional commit message such as `feat(frontend): show spinner on approve button in review submit dialog` and push.
