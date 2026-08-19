# Download files from the spec task workspace browser

## Summary
Add a Download action to the desktop workspace file browser so users can save task files locally. The new authorized streaming endpoint supports complete large and binary files while preserving the workspace browser's path-containment, ignored-file, and symlink protections.

## Testing
- `go test ./api/pkg/desktop -run 'TestWorkspaceFile' -count=1 -timeout=60s` — passed, including a new complete binary download test above the preview size limit.
- `go test ./api/pkg/server -run 'TestWorkspaceReviewHandlersSuite' -count=1 -timeout=60s` — passed.
- `git diff --check` — passed.
- Frontend type-check was attempted but could not run because frontend dependencies are not installed in the checkout (`tsc: not found`).
