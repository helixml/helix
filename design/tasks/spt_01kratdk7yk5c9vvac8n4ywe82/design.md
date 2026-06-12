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

## Implementation Notes (for future cloners)

- **Final diff is exactly the 3 lines in the design above.** No
  additional file touched — `isSubmitting` was already passed from
  `DesignReviewContent.tsx` via the React Query mutation.
- **Verification trick — no need to drive a real spec task through to
  design review.** Building review state requires onboarding + project
  + task + waiting for agent design generation, which is slow. Instead
  mount `ReviewSubmitDialog` directly via Vite's already-loaded module
  graph and screenshot. Pattern that worked from the chrome-devtools
  MCP `evaluate_script`:
  ```js
  const React = (await import('/node_modules/.vite/deps/react.js')).default;
  const ReactDOM = (await import('/node_modules/.vite/deps/react-dom_client.js')).default;
  const Mui = await import('/node_modules/.vite/deps/@mui_material.js');
  const Dialog = (await import('/src/components/spec-tasks/ReviewSubmitDialog.tsx')).default;
  // mount with isSubmitting=true and inspect / screenshot
  ```
  Notes: React and ReactDOM are CJS, so use `.default`; `@mui/material`
  is ESM, so destructure directly. Hash query param (`?v=…`) is
  optional and unstable per module — omit it.
- **Gotcha: `frontend/dist` is bind-mounted root-owned on a fresh
  workspace.** `yarn build` fails with `EACCES … mkdir
  'frontend/dist/external-libs'`. Fix is `sudo chown -R retro:retro
  frontend/dist`. Do NOT `rm -rf frontend/dist` (would break the
  bind-mount per `CLAUDE.md`). Use `rm -rf frontend/dist/*` instead.
  In dev mode (`FRONTEND_URL` unset) the build isn't needed for
  runtime — Vite HMR at port 8081 serves the source directly — but the
  build still has to pass for CI / prod-frontend mode.
- **MUI quirk worth knowing.** When `disabled` is combined with
  `variant="contained" color="success"`, MUI overrides the background
  to the disabled grey (`rgba(255,255,255,0.12)` in dark mode). The
  spinner with `color="inherit"` then takes the disabled text colour
  (also greyed). This matches every other in-flight button in the
  codebase (e.g. `SpecTaskActionButtons` Approve Implementation
  button) — it's the intended pattern, not a bug. Don't try to "fix"
  it by overriding the disabled background.
- **First yarn install on a new workspace** takes ~30s; subsequent
  `yarn tsc` is ~28s; full `yarn build` ~37s.
