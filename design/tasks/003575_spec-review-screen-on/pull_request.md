# Fix spec review text wrapping on mobile

## Summary
Remove the fixed minimum width from the spec review document container so rendered Markdown can shrink and wrap within narrow mobile viewports instead of being clipped beyond the right edge.

## Testing
- `yarn test src/components/spec-tasks/DesignReviewContent.test.tsx` — 5 tests passed.
- `yarn tsc` — passed.
- `git diff --check` — passed.
