# style(frontend): unify Review Spec & View PR button colours

## Summary

The "Review Spec" and "View Pull Request" action buttons on spec tasks rendered
in slightly different colours from the primary "Create Task" button, in both
light and dark mode. This standardises them on the brand `secondary`
teal/cyan so all primary actions look consistent.

## Changes

All in `frontend/src/components/tasks/SpecTaskActionButtons.tsx`:

- "Review Spec" button: `color="info"` → `color="secondary"` (inline + stacked).
- "View Pull Request" button: dropped the PR-state colour ternary
  (`success` for merged / `inherit` for closed / `secondary` for open) and use a
  constant `color="secondary"` (inline + stacked). PR state is still shown by the
  adjacent `PRStateBadge`, so no information is lost. Removed the now-unused
  `prState` local.
- Retinted the "Review Spec" pulse-glow from blue `rgba(41,182,246,…)` to the
  brand cyan `rgba(0,213,255,…)` so the glow matches the new button colour.

The multiple-PRs button variant already used `secondary`, so it was unchanged.
The "Reopen" button (done phase) is intentionally left as-is.

## Verification

- `tsc --noEmit` clean; `vite build` transformed all modules successfully.
- Verified live in the dev stack (light + dark mode): Review Spec matches the
  "New Task" colour, and a merged PR's button is now teal/cyan (was green) with
  the "merged" badge still displayed.

## Screenshots

![Review Spec button (light) matches New Task](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002190_review-spec-and-view/screenshots/01-review-spec-light.png)
![View PR button, merged state (light)](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002190_review-spec-and-view/screenshots/02-view-pr-merged-light.png)
![View PR button, merged state (dark)](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002190_review-spec-and-view/screenshots/03-view-pr-merged-dark.png)
