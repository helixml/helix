# Implementation Tasks: Prevent New Comment Form From Overlapping Existing Comment Bubbles on Spec Review Page

- [x] Reproduce the bug in the inner Helix at `localhost:8080`: confirmed deterministically from code — `commentFormPosition.y` is fed straight into `<InlineCommentForm yPos>` (DesignReviewContent.tsx:1566) without passing through the stacking algorithm at lines 706-805. Full live reproduction skipped because the inner Helix has zero existing spec tasks and generating one requires a multi-minute LLM cycle; bug path is clear from code review.
- [~] In `frontend/src/components/spec-tasks/InlineCommentForm.tsx`, convert the component to `React.forwardRef<HTMLDivElement>` (or accept an `outerRef` prop) so the parent can measure the form's rendered height — mirror the existing `commentRefs` pattern used for bubbles.
- [ ] In `frontend/src/components/spec-tasks/DesignReviewContent.tsx`, add a `commentFormRef = useRef<HTMLDivElement>(null)` and pass it down to `<InlineCommentForm>` (line 1564).
- [ ] In the stacking `useEffect` (lines 706-805), when `showCommentForm && selectedText`, build a pseudo-entry `{ id: "__new_comment_form__", baseY: commentFormPosition.y, height: commentFormRef.current?.offsetHeight ?? 220 }` and merge it into the `positions` array.
- [ ] Sort the merged `positions` array by `baseY` ascending before the overlap-resolution loop so whichever element is higher in the document wins its anchor slot.
- [ ] Add `showCommentForm`, `commentFormPosition.y`, and `selectedText` to the effect's dependency array (currently `[inlineCommentIds, activeTab, documentContent]`) so re-stacking triggers when the form opens, closes, or moves.
- [ ] Verify the `setCommentPositions` short-circuit (lines 777-783) still updates the map when the pseudo-entry is added/removed — adjust the comparison if needed so the form's appearance reliably re-flows bubbles.
- [ ] In the form render (line 1566), replace `yPos={commentFormPosition.y}` with `yPos={commentPositions.get("__new_comment_form__") ?? commentFormPosition.y}` so the form uses the resolved (collision-avoided) Y.
- [ ] Confirm cancel and submit paths still clear `showCommentForm`, and that bubbles re-stack without the pseudo-entry (no leftover gap).
- [ ] Build the frontend (`cd frontend && yarn build`) and verify the dev server hot-reloads cleanly with no TypeScript errors.
- [ ] End-to-end test in the inner Helix: walk through the reproduction steps from the design doc's Testing section (single-bubble + form, multi-bubble + form, cancel, narrow viewport).
- [ ] Take before/after screenshots and attach them to the PR description.
- [ ] Push the feature branch — the Helix platform creates the GitHub PR automatically when the user clicks "Open PR" in the UI. Do **not** run `gh pr create`.
- [ ] Write per-repo PR description at `pull_request_helix.md` in this task directory.
