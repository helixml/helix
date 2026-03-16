# Requirements: Comment Button Spinner

## User Story

As a user submitting a comment on a design review, I want the "Comment" button to show a loading spinner while my comment is being submitted, so I know the action is in progress and don't accidentally click it again.

## Acceptance Criteria

1. When the "Comment" button is clicked, it immediately shows a `CircularProgress` spinner inside/alongside the button label.
2. The button is disabled while submission is in progress (prevents double-submit).
3. Once the mutation completes (success or error), the spinner disappears and the button returns to its normal state.
4. The spinner matches the codebase convention: `<CircularProgress size={16} color="inherit" />` as `startIcon`.
