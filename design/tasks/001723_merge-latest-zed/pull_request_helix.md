# Bump ZED_COMMIT after upstream merge

## Summary

Update `ZED_COMMIT` in `sandbox-versions.txt` to the new fork HEAD SHA after the upstream Zed merge PR lands. This brings in 509 upstream commits including sidebar rework, agent panel improvements, and bug fixes.

## Changes

- Update `ZED_COMMIT` SHA in `sandbox-versions.txt` to point to the merged fork HEAD

## Test plan

- [ ] Rebuild Zed binary from new SHA
- [ ] E2E test: all 10 phases pass for `zed-agent`
- [ ] E2E test: all 10 phases pass for `claude` agent
