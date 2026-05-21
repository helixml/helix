# Bump ZED_COMMIT to merge of upstream Zed (task 002029)

## Summary

Bumps `ZED_COMMIT` in `sandbox-versions.txt` to point at the new merge of upstream Zed (`zed-industries/zed` `1399540715`, 261 commits / 10 days) on the Helix fork.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT` `fd26c1a113e69106faba1cabab821e7da983964b` → `8692f073b23fb86d457ee615c14940c036c2f76f`

The new Zed commit is the head of `helixml/zed` `feature/002029-merge-latest-zed` (paired Zed PR). It includes:

- The upstream merge (6 conflicts resolved; Helix surface preserved).
- Build fixes for upstream signature drift: `c3951af24f` session-directories changes to `agent_servers/acp.rs`; new `ThreadView::new` / `SessionCapabilities::new` / `ConversationView` field additions in `from_existing_thread`; `ContextServerStatus::ClientSecretRequired` exhaustiveness in the Helix UI state queries.
- The `supports_delete(&self)` → `supports_delete(&self, &App)` trait-signature migration across `agent_ui/src/acp/thread_history.rs` (forced by upstream `23231879cd`).
- The deliberate retirement of Critical Fix #10 (180s context-server timeout) per `e60a1b2789` — wrong diagnosis, reverted to upstream's 60s.
- Porting guide updated with the full Merge 002029 entry (incremental conflict resolutions, Pre-existing Breakage Repaired subsections, retired Critical Fix #10, commit-history table extension).

## Validation

- Zed E2E (zed-agent + claude, all 17 phases — including the Phase 17 live Claude process count gate that proves PR #56 Fix 1b draft-suppression survived the merge): green.
- All 10 surviving Helix Critical Fixes verified.
- Helix bypass markers in `extensions_ui.rs` retained (upstream `c84c22dab5` reshaped surrounding code but didn't subsume the bypass intent).
- See paired Zed PR for the full conflict-resolution trail.
