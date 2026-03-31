# Requirements: Bump sandbox-versions.txt on merge

## Problem

When changes are merged to main in the `zed` or `qwen-code` repos, `sandbox-versions.txt` in the Helix repo must be updated with the new commit hash. Without a reminder, this step is easily forgotten, causing CI to test against stale pinned commits.

## User Story

As a developer merging changes to `zed` or `qwen-code`, I want a clear reminder in CLAUDE.md to update `sandbox-versions.txt`, so that CI always builds from the correct commit.

## Acceptance Criteria

- CLAUDE.md in the Helix repo contains an explicit reminder: before merging a Zed or Qwen PR, first open a Helix PR to bump `sandbox-versions.txt`.
- The reminder makes clear the required order: **open the Helix PR first**, then merge the Zed/Qwen PR, then merge the Helix PR.
- The reason for this order is documented: if the Zed PR is merged first, the spec task system may mark the task done before the hash is actually bumped in Helix.
- The reminder includes the correct format: `ZED_COMMIT=<hash>` / `QWEN_COMMIT=<hash>`.
