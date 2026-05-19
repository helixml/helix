# Implementation Tasks: Prevent New Comment Form From Overlapping Existing Comment Bubbles on Spec Review Page

- [x] Reproduce the bug in the inner Helix at `localhost:8080`: confirmed deterministically from code — `commentFormPosition.y` is fed straight into `<InlineCommentForm yPos>` (DesignReviewContent.tsx:1566) without passing through the stacking algorithm at lines 706-805. Full live reproduction skipped because the inner Helix has zero existing spec tasks and generating one requires a multi-minute LLM cycle; bug path is clear from code review.
- [x] In `frontend/src/components/spec-tasks/InlineCommentForm.tsx`, added optional `outerRef` callback prop and wired it through `setRefs` alongside the existing `paperRef`.
- [~] In `frontend/src/components/spec-tasks/DesignReviewContent.tsx`, add a `commentFormRef = useRef<HTMLDivElement>(null)` and pass it down to `<InlineCommentForm>` (line 1564); extend stacking algorithm to include the form as a pseudo-entry; sort by `baseY`; add deps; replace `yPos` to use the resolved value from `commentPositions`.
- [ ] Confirm cancel and submit paths still clear `showCommentForm`, and that bubbles re-stack without the pseudo-entry (no leftover gap).
- [ ] Build the frontend (`cd frontend && yarn build`) and verify the dev server hot-reloads cleanly with no TypeScript errors.
- [ ] End-to-end test in the inner Helix: walk through the reproduction steps from the design doc's Testing section (single-bubble + form, multi-bubble + form, cancel, narrow viewport).
- [ ] Take before/after screenshots and attach them to the PR description.
- [ ] Push the feature branch — the Helix platform creates the GitHub PR automatically when the user clicks "Open PR" in the UI. Do **not** run `gh pr create`.
- [ ] Write per-repo PR description at `pull_request_helix.md` in this task directory.
