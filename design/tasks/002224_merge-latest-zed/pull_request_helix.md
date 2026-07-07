# Bump ZED_COMMIT to 002224 upstream merge (ACP 1.0.1)

## Summary

Points `sandbox-versions.txt` `ZED_COMMIT` at the Helix-fork branch that merges 289 upstream `zed-industries/zed` commits (through `872ca8fef5`) and the ACP `agent-client-protocol` 0.14.0 → 1.0.1 bump. No Helix Go/frontend changes — this is purely the Zed pin bump for CI to build the sandbox from the new fork HEAD.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT` `9546054e68…` → `2d1f97400bfe19122557e0e9e3825fb6970d78fa` (final 002224 zed HEAD, includes the merge, the ACP 1.0 compile-fixes, and the e2e test-server `go mod tidy`).
- Merged latest `origin/main` into the branch (resolved the `sandbox-versions.txt` overlap with PR #66's 002228 pin — kept the 002224 HEAD, which is a superset of `548da160ce`).

## Notes

- The Zed side is a separate PR (`helixml/zed` `feature/002224-merge-latest-zed`). Per the CLAUDE.md ordering, this Helix PR carries the bumped hash; merge the Zed PR first, then this one.
- The Zed WebSocket-sync E2E `zed-agent` round passes all 17 phases against the new binary; the `claude` round's new Phase 17 (from PR #66) is validated in CI with the real Anthropic model.

Release Notes:

- N/A
