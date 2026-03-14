# Implementation Tasks

- [ ] In `DesignReviewContent.tsx` (~line 1040), wrap the `Chip` (inside its `Tooltip`) with a `Box` that has `sx={{ display: { xs: 'none', sm: 'flex' } }}` to hide it on mobile
- [ ] On the `Typography` showing `git_pushed_at` (~line 1049), add `display: { xs: 'none', sm: 'block' }` to its existing `sx` prop to hide it on mobile
- [ ] Build the frontend (`cd frontend && yarn build`) and verify no TypeScript errors
- [ ] Visually verify on a narrow viewport that the chip and timestamp are hidden while the share and comment icons remain
