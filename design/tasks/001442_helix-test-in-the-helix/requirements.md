# Requirements: Remove Broken File Viewer Link from `helix test`

## Background

The `helix test` CLI command runs tests defined in helix.yaml and generates HTML/JSON/Markdown reports. After tests complete, it:
1. Prints a link to view results in the file viewer: `View results at: {url}/files?path=/test-runs/{testID}`
2. Auto-launches that link in the default browser (if in a graphical environment)

The file viewer URL is now broken, so this link is useless and the auto-launch is annoying.

## User Stories

**As a developer running `helix test`:**
- I want test results to complete without auto-launching a broken URL in my browser
- I still want to see local file paths where results are written (which already works)

## Acceptance Criteria

1. `helix test` no longer calls `openBrowser()` after writing results
2. `helix test` no longer prints the "View results at:" line with the broken file viewer URL
3. Local file paths are still printed (JSON, HTML, summary files)
4. No other behavior changes - tests still run, reports still generate, files still upload

## Out of Scope

- Fixing the file viewer itself
- Adding a new `--no-browser` flag (not needed since we're removing the feature entirely)
- Changing the upload behavior (files still upload to filestore)