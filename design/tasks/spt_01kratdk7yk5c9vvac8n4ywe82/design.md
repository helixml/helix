# Design: Spinner on Approve Button in Review Submit Dialog

## Scope

Single-file change in the Helix frontend:
`frontend/src/components/spec-tasks/ReviewSubmitDialog.tsx`.

## Current State

`ReviewSubmitDialog.tsx` lines 48–55:

```tsx
<Button
  variant="contained"
  color={decision === 'approve' ? 'success' : 'warning'}
  onClick={onSubmit}
  disabled={isSubmitting}
>
  {decision === 'approve' ? 'Approve' : 'Submit Feedback'}
</Button>
```

The `isSubmitting` prop is already passed from
`DesignReviewContent.tsx` (sourced from the React Query mutation
`submitReviewMutation.isPending`), so all wiring is in place — the only
gap is visual.

## Change

Add a `startIcon` and conditional label. After the change:

```tsx
<Button
  variant="contained"
  color={decision === 'approve' ? 'success' : 'warning'}
  onClick={onSubmit}
  disabled={isSubmitting}
  startIcon={
    isSubmitting ? <CircularProgress size={16} color="inherit" /> : undefined
  }
>
  {isSubmitting
    ? (decision === 'approve' ? 'Approving…' : 'Submitting…')
    : (decision === 'approve' ? 'Approve' : 'Submit Feedback')}
</Button>
```

Add `CircularProgress` to the MUI imports at the top of the file.

## Why this pattern

The codebase already has a consistent spinner-in-button pattern using
MUI's `startIcon` + `CircularProgress`. Matching it:

- avoids reinventing styling (size, colour, vertical alignment),
- keeps the diff to roughly three lines,
- ensures the spinner colour inherits from the contained-button text
  colour, so it stays visible against both `success` (green) and
  `warning` (orange) backgrounds without per-variant tweaks.

Reference implementations already in the repo:

- `frontend/src/components/tasks/SpecTaskActionButtons.tsx` — "Approve
  Implementation" button.
- `frontend/src/components/app/AddMcpSkillDialog.tsx` — dialog
  confirm pattern.
- `frontend/src/components/app/DuplicateDialog.tsx` — secondary
  dialog confirm pattern.

## Key Decisions

1. **`size={16}` not `14` or `20`.** Matches the secondary-dialog
   examples (`AddMcpSkillDialog`, `DuplicateDialog`) — visually
   appropriate for a default-sized MUI Button without crowding.
2. **`color="inherit"`.** The button is contained with `success` /
   `warning`, so the text is white; an inherited-colour spinner stays
   white and visible. Avoids the default primary blue that would
   disappear against the green/orange background.
3. **Use ellipsis character `…` not three dots `...`.** Already the
   convention in other "-ing…" buttons in the codebase.
4. **Use `startIcon`, not wrapping the spinner inside the label.**
   `startIcon` is what every other example uses and keeps spacing
   consistent with MUI's button conventions.
5. **No new prop.** The change relies entirely on the existing
   `isSubmitting` prop. No parent (`DesignReviewContent.tsx`) changes
   needed.
6. **No new tests required.** This is a presentational tweak driven by
   an existing prop already covered by parent integration paths.
   `yarn build` confirms type-correctness; UI verification belongs in
   the manual test below.

## Testing

- `cd frontend && yarn build` — must pass.
- Manual smoke test in the inner Helix dev environment:
  1. Log in to `http://localhost:8080`, complete onboarding if needed.
  2. Open a spec task that has a pending design review.
  3. Open the review and click "Approve Design" to surface the dialog.
  4. Optionally enter a comment, then click "Approve" — confirm the
     button shows a spinner and "Approving…" label until the mutation
     settles, then the dialog closes.
  5. Repeat for "Request Changes" — confirm "Submit Feedback" /
     "Submitting…" path also works.

## Risks

Minimal. The change is additive, visual-only, and gated by an existing
prop. The button remains functionally identical when `isSubmitting` is
false. The one thing to watch is that `startIcon={undefined}` (rather
than omitting the prop) does not introduce extra left padding — MUI
treats `undefined` startIcon as absent, so spacing stays clean.
