# Add inline comments to the workspace diff viewer

## Summary
Reuse the workspace file browser's inline comment experience in the diff viewer. Reviewers can select added or deleted lines, leave comments with the relevant file, range, side, and quoted content attached, and send those comments through the existing review prompt flow. Pause live diff polling while text is selected or a comment draft is open so background refreshes do not interrupt selection or editing.

## Testing
- Passed 17 focused Vitest tests covering diff comments, workspace review comment serialization, the shared comment form, and polling suspension during selection.
- Passed the full frontend TypeScript project check with `yarn tsc --pretty false`.
- Passed the production frontend build with `yarn build`.
