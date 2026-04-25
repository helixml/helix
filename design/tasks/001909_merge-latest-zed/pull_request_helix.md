# Bump ZED_COMMIT to 001909 merge

## Summary

Bumps `sandbox-versions.txt` `ZED_COMMIT` to point at the head of the 001909 merge branch in the Zed fork. Companion to the Zed PR (which merges 86 upstream commits + 3 carry-over fixes).

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT=...` → new merge commit SHA

## Why

Per `CLAUDE.md` "Bumping sandbox-versions.txt after Zed or Qwen changes" — this PR must be opened *before* the Zed PR is pushed/merged so the spec task system doesn't close the task with CI pinned at the wrong commit.

Release Notes:

- N/A
