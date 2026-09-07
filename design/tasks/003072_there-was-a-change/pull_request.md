# Highlight changed files in the workspace explorer

## Summary
Make changed files immediately recognizable in the chat-view workspace explorer with a tinted row background and colored left rail. Both treatments reuse the file tree's native Git status colors so added, modified, deleted, and renamed files remain consistent with its built-in indicators. Selected changed files retain the tree's native selection background while keeping the status rail visible.

## Testing
Ran all workspace inspector test suites: 12 files and 69 tests passed. Ran the frontend TypeScript build check successfully.
