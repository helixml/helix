# Implementation Tasks

## Left panel tab strip

- [ ] Add `leftTab: 'chat' | 'spec'` state to `SpecTaskDetailContent.tsx`
- [ ] Compute default tab: `'spec'` when `task.phase` is `'planning'` or `'review'` AND `hasDesignReview` is true; otherwise `'chat'`
- [ ] Render a two-tab strip ("Chat" / "Spec") at the top of the left panel, only showing "Spec" tab when `hasDesignReview` is true
- [ ] Wire "Spec" tab `onClick` to call the existing `handleReviewSpec()` function
- [ ] On mount (or when `task.phase`/`hasDesignReview` change), set `leftTab` to the computed default and auto-trigger `handleReviewSpec()` if default is `'spec'`
- [ ] Keep "Chat" tab rendering `EmbeddedSessionView` exactly as before

## Back navigation from spec review

- [ ] In `DesignReviewContent.tsx`, accept an optional `onBack?: () => void` prop and render a "← Back to issue" / close button when it is provided
- [ ] In `SpecTaskReviewPage.tsx`, pass `onBack={() => router.back()}` (or navigate to the task detail route) into `DesignReviewContent`
- [ ] Verify that in workspace/TabsView mode the existing tab bar is sufficient and no additional back button is needed

## Polish & edge cases

- [ ] When `hasDesignReview` becomes false (spec not yet generated), ensure the Spec tab is hidden and `leftTab` resets to `'chat'`
- [ ] Confirm the "Review Spec" button in `SpecTaskActionButtons.tsx` still works and is not broken by the above changes (no change expected, just verify)
- [ ] Smoke-test: click a task in the Spec Review column → Spec tab selected by default, spec opens → click back → return to issue chat view
