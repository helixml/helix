# Design: Prevent New Comment Form From Overlapping Existing Comment Bubbles on Spec Review Page

## Root Cause

`DesignReviewContent.tsx` has a stacking algorithm in the
`useEffect` at lines 706-805 that walks `inlineComments`, finds each
bubble's anchor Y via `findQuotedTextPosition(quoted_text)`, measures the
bubble's actual rendered height via `commentRefs.current.get(id).offsetHeight`,
then pushes each bubble down so that no two overlap (minGap = 10px).
The result is written to `commentPositions: Map<id, y>` and consumed by
`InlineCommentBubble` (line 1524).

The new-comment editor (`InlineCommentForm`, line 1564) is positioned
independently using `commentFormPosition.y`, which is set directly from
the raw selection rect (line 936) inside `handleTextSelection`. The form
is **never an input to the bubble-stacking algorithm**, and the
algorithm's output never feeds back into the form's position.

Both elements live in the same `position: absolute; left: 820px;
width: 300px` column on wide viewports. The bubble has `zIndex: 10`, the
form `zIndex: 20`. So when their Y ranges overlap, the form renders
visibly on top of the bubble — exactly the messy UI the user is seeing.

## Approach

Extend the existing stacking algorithm so that the open form participates
as if it were one more item to lay out. This keeps a single source of
truth for vertical layout in the right column.

Concretely:

1. Treat the open form as a "pseudo-comment" entry with:
   - a synthetic id (e.g. `"__new_comment_form__"`),
   - `baseY = commentFormPosition.y`,
   - `height = paperRef.current?.offsetHeight ?? FORM_FALLBACK_HEIGHT`
     (the form already has a ref; we pass it up or measure via a new
     ref held in `DesignReviewContent`).
2. Include this entry in the `positions` array before running the
   overlap-resolution loop. Sort the array by `baseY` so that whichever
   item is higher in the document wins its preferred slot, and lower
   items get pushed down. (The current code already implicitly assumes
   `inlineComments` is ordered top-to-bottom by anchor, which is
   produced by `findQuotedTextPosition` over `quoted_text` matches; we
   keep that property by inserting the form entry in the right place.)
3. The resolved Y for the form entry is the value the form should
   render at — replace `yPos={commentFormPosition.y}` (line 1566) with
   `yPos={commentPositions.get("__new_comment_form__") ?? commentFormPosition.y}`.
4. Recompute when `showCommentForm`, `commentFormPosition.y`, or the
   form's measured height changes. Add these to the effect's dependency
   array (alongside the existing `inlineCommentIds`, `activeTab`,
   `documentContent`).

This approach is symmetric: in cases where the existing bubble is below
the new selection's natural Y, the bubble gets pushed down to make room
for the form instead. That falls out naturally from sorting by `baseY`.

## Why Not Alternative Approaches

- **Close the existing bubble when opening the form** — wrong; the user
  legitimately wants to see prior comments while authoring a new one.
- **Just bump the form's Y by the nearest bubble's bottom** — works for
  one bubble but breaks down with multiple bubbles, and duplicates the
  collision logic that already exists.
- **Switch the right column to flexbox / sticky stacking** — much bigger
  change; the document/anchor coupling (form should appear near the
  selected text) would be lost.

## Key Files

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx`
  - state: `commentPositions` (line 137), `commentFormPosition` (line 118),
    `showCommentForm` (line 115)
  - stacking effect: lines 706-805
  - form render: lines 1564-1579
  - selection handler: lines 902-952
- `frontend/src/components/spec-tasks/InlineCommentForm.tsx`
  - already exposes a `paperRef`; if we measure from the parent we can
    forward this ref out through a new prop, or attach a sibling ref in
    `DesignReviewContent`. Forwarding via `React.forwardRef` is cleaner.

## Constraints and Gotchas (notes for implementer)

- Heights for new bubbles aren't known until after they render — the
  existing code handles this by defaulting to `250` when no ref exists
  (line 736) and re-running the effect when measurements change. Use
  the same trick for the form: default to a sensible fallback (~220px
  matches the form's typical rendered size with a 3-line TextField).
- The effect uses `inlineCommentIds` as a dependency to avoid recomputing
  on every comment-body change; include `showCommentForm` and
  `commentFormPosition.y` so the form's open/close and reposition
  trigger a re-stack.
- Cancel/submit paths (lines 968-975, 1571-1576) already set
  `showCommentForm = false`. The dependency on `showCommentForm` will
  cause the effect to re-run and re-stack bubbles without the form
  entry, naturally cleaning up the gap.
- Do not break the existing `prev` short-circuit in the
  `setCommentPositions` updater (lines 777-783) — the form entry's
  presence/absence must change the map size or values, otherwise the
  bubbles won't reflow.

## Testing

Use the inner Helix instance per `helix/CLAUDE.md` for end-to-end
verification:

1. Register/login at `localhost:8080`, create a project, create a spec
   task, advance to the design review stage.
2. Add a comment on an early paragraph. Confirm the bubble renders to
   the right.
3. Select text in the **same paragraph or one immediately following**
   (so the new selection's Y falls within the existing bubble's
   vertical range). Confirm the new comment form appears below the
   existing bubble with visible spacing, not overlapped.
4. Submit a second comment, then open a third near the first two.
   Confirm three-way stacking with the form on the bottom.
5. Cancel the form. Confirm the two bubbles snap back to their natural
   stacked positions (no leftover gap).
6. Resize the window below 1000px and repeat: confirm the narrow-view
   layout still renders bubbles inline and the form as a bottom-sheet
   with no overlap regressions.
