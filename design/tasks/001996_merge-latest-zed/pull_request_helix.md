# Bump ZED_COMMIT to merge of upstream Zed (task 001996)

## Summary

Bumps `ZED_COMMIT` in `sandbox-versions.txt` to point at the new merge of upstream Zed (`zed-industries/zed` `8bdd78e023`, 127 commits / 3 days) on the Helix fork.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT` `fe8f4f4e3f0fb7c0cb51e9c8028ca0c13a8252cb` → `a767007e53304b077eaee5d1872a5631e2ada207`

The new Zed commit is the head of `helixml/zed` `feature/001996-merge-latest-zed` (paired Zed PR). It includes the upstream merge, a `BaseView::Terminal` match-arm fix in the Helix UI state query, a Phase 13 race fix in the cancellation handler (turn_cancelled now wins the race against message_completed), and the porting-guide update.

## Validation

- Zed E2E (zed-agent + claude, all 14 phases) green
- All Helix Critical Fixes verified
- See paired Zed PR for the full conflict-resolution trail
