# Bump ZED_COMMIT to 42b8107379 (001980 upstream merge)

## Summary

Pulls in the 001980 Zed upstream merge: 172 upstream commits over 10 days plus the Helix-side carry-over fixes needed to keep the WebSocket sync layer compiling and the test suite green.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT=f5fab9785759… → 42b81073797…`

## Verification

The Zed branch was built and tested locally before this bump:

- `./stack build-zed dev` succeeds — clean compile with `external_websocket_sync` feature
- `external_websocket_sync` E2E (Docker, both `zed-agent` and `claude` rounds): **24/24 phases PASSED**
- All 9 critical fixes preserved
- All Helix-specific surface preserved (callbacks, accessors, ACP beta override, built-in agent hiding, enterprise TLS skip, `--allow-multiple-instances`, `debug-embed`)
- PRs #44–#47 baked through unchanged

## Pairing

Companion PR on the Zed fork: `feature/001980-merge-latest-zed` (commit `42b8107379`). See `portingguide.md` §"Merge 001980" for per-conflict resolution rationale.

Per `CLAUDE.md` ordering rule, this Helix PR opens **before** the Zed PR is merged.
