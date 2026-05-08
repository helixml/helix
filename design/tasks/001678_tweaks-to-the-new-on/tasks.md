# Implementation Tasks

All changes in `frontend/src/components/spec-tasks/DesignReviewContent.tsx`.

- [x] In the hover button `onClick`, create a `Range` over `hoveredElementRef.current` and assign it to `savedRangeRef.current` before setting `showCommentForm = true`, so the existing `useEffect` applies the pseudo-highlight
- [x] Add `onMouseMove` to the outer scroll container (the Box with `onMouseLeave`) that clears `hoverButtonPosition` when the cursor x-position exceeds the button's right edge (`containerWidth/2 + 432px`)
- [x] Adjust the `::highlight(comment-highlight)` GlobalStyles rule (line ~1116) so the highlight is visible across code blocks: dropped the conflicting `color: #000` override that competed with Prism's inline syntax-token colours. Verified in-browser that `background-color: #b3d7ff` now paints across `<pre>` tokens with token colours preserved (see screenshot 02)
- [x] At the top of the inner Box's `onMouseMove` handler, check if `e.target` is contained within any `commentRefs.current` entry; if so, clear `hoverButtonPosition` and `hoveredElementRef.current` and return early
- [x] Run `cd frontend && yarn build` to verify no TypeScript errors before committing
- [x] Manual browser test in the inner Helix: registered/logged in, created spec task with code block via SQL, navigated to design review at `/orgs/{org}/projects/{prj}/tasks/{task}/review/{rev}`. Verified (a) clicking hover button highlights paragraph (screenshot 04), (b) moving cursor past button right edge hides it (programmatic dispatch confirmed `stillVisibleAtButton:true` when cursor inside button, button cleared when moved to x=1100 past 1040 right edge), (c) selection across code block highlights correctly (screenshot 02), (d) hovering over comment panel hides button (screenshot 05)
