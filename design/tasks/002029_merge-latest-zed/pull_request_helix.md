# Bump ZED_COMMIT to merge of upstream Zed (task 002029)

## Summary

Bumps `ZED_COMMIT` in `sandbox-versions.txt` to point at the (two-round-stacked) merge of upstream Zed (`zed-industries/zed` `13e7c11768`, 548 commits / 13 days) on the Helix fork.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT` `fd26c1a113e69106faba1cabab821e7da983964b` → `fb97e2cf952c467f9101867e5eedbd42ee5c44df`

The new Zed commit is the head of `helixml/zed` `feature/002029-merge-latest-zed` (paired Zed PR). It includes:

- **Round 1** upstream merge (261 commits to `1399540715`; 6 conflicts resolved; Helix surface preserved).
- **Round 1** build fixes for upstream signature drift: `c3951af24f` session-directories changes to `agent_servers/acp.rs`; new `ThreadView::new` / `SessionCapabilities::new` / `ConversationView` field additions in `from_existing_thread`; `ContextServerStatus::ClientSecretRequired` exhaustiveness in the Helix UI state queries.
- **Round 1** `supports_delete(&self)` → `supports_delete(&self, &App)` trait-signature migration across `agent_ui/src/acp/thread_history.rs` (forced by upstream `23231879cd`).
- **Round 1** deliberate retirement of Critical Fix #10 (180s context-server timeout) per `e60a1b2789` — wrong diagnosis, reverted to upstream's 60s.
- **Round 2** upstream merge (a further 287 commits to `13e7c11768`; zero manual conflicts — auto-merged via `ort`; Helix surface verified via grep).
- **Round 2** build fix: `code_span_resolver` field added to `ConversationView` and as a positional arg to `ThreadView::new` (upstream `cfd0461b5a`) — repaired in `from_existing_thread`.
- Porting guide updated with both `## Merge 002029 (2026-05-21)` and `## Merge 002029-extension (2026-05-25)` entries.

## Validation

- Zed E2E (zed-agent + claude, all 17 phases — including the Phase 17 live Claude process count gate that proves PR #56 Fix 1b draft-suppression survived the merge): green after both rounds.
- All 10 surviving Helix Critical Fixes verified.
- Helix bypass markers in `extensions_ui.rs` retained (upstream `c84c22dab5` reshaped surrounding code but didn't subsume the bypass intent).
- See paired Zed PR for the full conflict-resolution trail.
