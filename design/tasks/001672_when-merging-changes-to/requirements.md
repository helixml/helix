# Requirements: Bump sandbox-versions.txt on merge

## Problem

When changes are merged to main in the `zed` or `qwen-code` repos, `sandbox-versions.txt` in the Helix repo must be updated with the new commit hash. Without a reminder, this step is easily forgotten, causing CI to test against stale pinned commits.

## User Story

As a developer merging changes to `zed` or `qwen-code`, I want a clear reminder in CLAUDE.md to update `sandbox-versions.txt`, so that CI always builds from the correct commit.

## Acceptance Criteria

- CLAUDE.md in the Helix repo contains an explicit reminder to update `sandbox-versions.txt` after merging changes to `zed` or `qwen-code` main branches.
- The reminder includes the correct format: `ZED_COMMIT=<hash>` / `QWEN_COMMIT=<hash>`.
- The reminder is placed in a visible, logical location (e.g., the Build Pipeline or CI section).
