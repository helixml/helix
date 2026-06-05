# fix(frontend): include new comment form in spec-review bubble stacking

## Summary

On the spec review page, opening a new inline comment near an existing
comment caused the new `InlineCommentForm` to render directly on top of
the existing `InlineCommentBubble` — both elements share the same
`position: absolute; left: 820px; width: 300px` column on wide
viewports.

Root cause: the bubble-stacking algorithm in `DesignReviewContent.tsx`
(the `useEffect` at the old line 706) computed collision-avoided
positions for `inlineComments` only. The form's Y was fed directly
from `commentFormPosition.y` (raw selection rect) and never
participated in collision resolution.

Fix: include the open form as a pseudo-entry (sentinel id
`__new_comment_form__`) in the same stacking algorithm. Sort entries
by `baseY` so whichever element is higher in the document wins its
preferred slot; the other gets pushed down past it.

## Changes

- `frontend/src/components/spec-tasks/InlineCommentForm.tsx` — added
  an optional `outerRef` callback prop so the parent can measure the
  form's rendered height.
- `frontend/src/components/spec-tasks/DesignReviewContent.tsx`
  - new `commentFormRef`, `commentFormMeasureTick` state, and stable
    `handleCommentFormRef` callback that bumps the tick on mount /
    unmount so the algorithm re-runs with the real measured height
    (fallback 220 px before mount);
  - stacking `useEffect` now appends a `{ id: NEW_COMMENT_FORM_KEY,
    baseY: commentFormPosition.y, height }` pseudo-entry when the form
    is open on a wide viewport, sorts entries by `baseY`, then runs
    the existing overlap-resolution loop unchanged;
  - deps extended with `showCommentForm`, `selectedText`,
    `commentFormPosition.y`, `commentFormMeasureTick`,
    `isNarrowViewport` so opening / moving / closing the form
    triggers a re-stack;
  - form's `yPos` prop now reads from
    `commentPositions.get(NEW_COMMENT_FORM_KEY)` (with raw-selection
    fallback), so it inherits the collision-avoided position.

Narrow-viewport behaviour is intentionally untouched — bubbles render
inline (`position: relative`) and the form is a bottom-sheet
(`position: fixed`), so collision math is irrelevant there.

## Testing

- `yarn tsc` — clean (no type errors).
- `vite build` — transformed all 21,104 modules successfully; the
  final dist-write step failed only due to `frontend/dist`
  bind-mount permissions in this dev environment (unrelated to the
  change).
- Inner-Helix Vite HMR (`helix-frontend-1`, port 8081) reloaded both
  modified files with no errors.
- Live end-to-end click-through deferred: the inner Helix has zero
  existing spec tasks and seeding one requires a multi-minute LLM
  generation cycle. Please verify on a real spec review session that
  opening a second comment near an existing bubble no longer
  overlaps it (and that cancel / submit cleanly re-stack the
  bubbles).

## Design docs

See
[`helix-specs/design/tasks/002037_fix-messy-ui-on-the-spec/`](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002037_fix-messy-ui-on-the-spec)
for full requirements, design rationale, and implementation notes.
