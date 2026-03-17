# Implementation Tasks

## Left panel tab strip

- [~] Add `leftTab: 'chat' | 'spec'` state to `SpecTaskDetailContent.tsx`
- [~] Compute default tab: `'spec'` when `task.status` is `spec_review` or `spec_revision` AND `design_docs_pushed_at` exists; otherwise `'chat'`
- [~] Render a two-tab strip ("Chat" / "Spec") at the top of the left panel, only showing "Spec" tab when `design_docs_pushed_at` exists
- [~] Wire "Spec" tab `onClick` to call the existing `handleReviewSpec()` function
- [~] On mount (or when task status/design_docs_pushed_at change), auto-trigger `handleReviewSpec()` if task is in spec_review/spec_revision with design docs
- [~] Keep "Chat" tab rendering `EmbeddedSessionView` exactly as before

## Back navigation from spec review

- [ ] In `DesignReviewContent.tsx`, accept an optional `onBack?: () => void` prop and render a "← Back to issue" / close button when it is provided
- [ ] In `SpecTaskReviewPage.tsx`, pass `onBack={() => router.back()}` (or navigate to the task detail route) into `DesignReviewContent`
- [ ] Verify that in workspace/TabsView mode the existing tab bar is sufficient and no additional back button is needed

## Polish & edge cases

- [ ] When `hasDesignReview` becomes false (spec not yet generated), ensure the Spec tab is hidden and `leftTab` resets to `'chat'`
- [ ] Confirm the "Review Spec" button in `SpecTaskActionButtons.tsx` still works and is not broken by the above changes (no change expected, just verify)
- [ ] Smoke-test: click a task in the Spec Review column → Spec tab selected by default, spec opens → click back → return to issue chat view
