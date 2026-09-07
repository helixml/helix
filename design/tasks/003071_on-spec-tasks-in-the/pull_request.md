# Fix diff comments in spec task chat

## Summary
Prevent the workspace diff viewer from rerendering between pointer down and pointer up when a reviewer clicks the comment gutter action. This preserves the diff library's active interaction so the comment editor opens correctly from spec task chat.

## Testing
Ran the focused WorkspaceDiffSurface test suite (10 tests passed) and the full frontend TypeScript compilation successfully.
