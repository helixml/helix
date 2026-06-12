# Show spinner on Approve button in review submit dialog

## Summary

When a user approves a design review and is prompted for an optional
comment, the confirmation button used to silently disable on click —
no visual feedback during the multi-second mutation. Users could not
tell whether their click was registered, sometimes clicking again or
assuming the app had hung.

This PR adds a `CircularProgress` spinner inside the button and toggles
the label between "Approve" / "Submit Feedback" (idle) and "Approving…"
/ "Submitting…" (in flight), matching the same pattern already used
across the rest of the app (`SpecTaskActionButtons`, `AddMcpSkillDialog`,
`DuplicateDialog`, …).

## Changes

- `frontend/src/components/spec-tasks/ReviewSubmitDialog.tsx`
  - Import `CircularProgress` from `@mui/material`.
  - Add `startIcon={isSubmitting ? <CircularProgress size={16} color="inherit" /> : undefined}` to the primary action button.
  - Conditional label: "Approving…" / "Submitting…" while `isSubmitting`, else the original "Approve" / "Submit Feedback".

No backend, prop, or wiring changes — the dialog already received
`isSubmitting` from `DesignReviewContent.tsx` via the React Query
mutation.

## Screenshots

| State | |
|-------|--|
| Resting (approve) | ![Resting](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kratdk7yk5c9vvac8n4ywe82/screenshots/00-approve-resting.png) |
| Submitting (approve) | ![Approving](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kratdk7yk5c9vvac8n4ywe82/screenshots/01-approving-spinner.png) |
| Submitting (request changes) | ![Submitting](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kratdk7yk5c9vvac8n4ywe82/screenshots/02-submitting-spinner.png) |

## Verification

- `cd frontend && yarn tsc` — passes.
- `cd frontend && yarn build` — passes.
- Mounted `ReviewSubmitDialog` live via Vite HMR with `isSubmitting=true`
  for both `decision: 'approve'` and `decision: 'request_changes'`;
  confirmed the button is disabled, labelled "Approving…" /
  "Submitting…", and renders the `MuiCircularProgress` SVG inline.
