# Add merge-order warning for sandbox-versions.txt to CLAUDE.md

## Summary

When Zed or Qwen changes are merged, `sandbox-versions.txt` must be bumped in this repo. Without a reminder, the hash update gets missed — especially because the spec task system closes tasks when all PRs are merged, which can happen before the Helix PR exists.

## Changes

- Added a `CRITICAL: Bumping sandbox-versions.txt` section to CLAUDE.md under Build Pipeline, documenting the required merge order: open the Helix PR first, then merge the Zed/Qwen PR, then merge the Helix PR.
