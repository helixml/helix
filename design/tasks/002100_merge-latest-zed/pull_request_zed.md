# Merge latest upstream Zed (25 commits, 2026-06-12 → 2026-06-15)

## Summary

Brings the Helix fork of Zed up to date with `zed-industries/zed` HEAD (`a31d3505da`), absorbing 25 upstream commits over 3 days — the **smallest catch-up window** of any merge in this series. **Near-clean merge**: only one trivial conflict (both-sides-added-a-field on `RemoteSettingsContent`), zero Helix-side signature-drift repairs needed, build green on first attempt, full E2E green for both `zed-agent` and `claude` personalities.

This is the **second consecutive** upstream merge that required **zero** "Pre-existing Breakage Repaired" entries (002077 was the first).

## Changes

- `Merge remote-tracking branch 'upstream/main'` (`0098823efa`) — 25 upstream commits including: editor bracket-splitting in line comments (`cccc7b2d44`), `objc2-app-kit` 0.3 → 0.3.2 with feature widening, GPUI macOS traffic-light hitbox fix (`138139f830`), audio phantom-presence in channels fix (`e5b6041e9d`), `multi_buffer` `range_to_buffer_ranges` micro-optimisation (`b6c7496aea`), repl notebook-cell kernel error display (`26a355b11d`), `git stash list` per-file-save optimisation (`a31d3505da`), `dev_container` BuildKit toggle (`26fc42721a`) and `$VAR` expansion (`832ab56db8`), agent shell-hang fix on syntax errors (`c578f4d12b`), extension_ui chip-filter restructure to hide agent servers (`f39cf25c0b`), and a `[patch.crates-io] async-process` entry to allow reaper reuse (`d4cc8d2409`).
- `docs(porting): add Merge 002100 entry` (`952f59f2d6`) — new `## Merge 002100 (2026-06-15)` section in `portingguide.md` documenting the single conflict, the auto-merge survival check across all critical fixes / PR #50 / PR #55 / PR #56 / PR #60 surface, and the risks that were considered in planning and turned out to be non-issues.
- `docs(porting): record 002100 build+E2E results` (`5ed995947e`) — fills in the Validation section after build + E2E completed green.

### Conflicts resolved

1. `crates/settings_content/src/settings_content.rs` — **both sides added a field on `RemoteSettingsContent`**. Helix's `suggest_dev_container: Option<bool>` (drives the `dev_container_suggest.rs` early-return guard) and upstream's `dev_container_use_buildkit: Option<bool>` from `26fc42721a` "dev_container: Support the classic Docker builder via a setting (#59288)". Kept both — independent settings, no semantic overlap.

All other files auto-merged cleanly, including the `Cargo.toml` `objc2` / `objc2-app-kit` upgrades + the new `[patch.crates-io] async-process` entry, three workflow YAMLs, and the two upstream commits in Helix-touched files (`agent_panel.rs` menu-link removal `1e017d04b9`, `extensions_ui.rs` chip-filter restructure `f39cf25c0b`).

### Helix surface — auto-merge survival check

Zero upstream churn in `acp_thread/`, `agent/src/`, `workspace.rs`, `zed/src/main.rs`, `title_bar/`, `feature_flags/`, `agent_servers/`, `external_websocket_sync/`, or `agent_settings/` — so the "must survive" surface is by-construction untouched. Verified intact:

- Critical Fixes #1, #3, #6, #8, #9, #11 — intact at the same line numbers as 002077.
- PR #50 `session_creation_chain` + `_settings_subscription` — intact at `agent_servers/src/acp.rs:438-439`.
- PR #55 streaming-reveal `EntryUpdated` emit — intact (16 occurrences in `acp_thread.rs`).
- PR #56 Fix 1a deferred `UserCreatedThread` — intact in `external_websocket_sync/src/thread_service.rs`.
- PR #56 Fix 1b cfg-gated early return — intact and verified as the FIRST statement of `BaseView::Uninitialized` at `agent_panel.rs:5420-5425`. `1e017d04b9`'s `Rules Library` menu deletion landed at line ~5690 in a different region.
- PR #60 `ede_diagnostic` retry loop — intact at `thread_service.rs:1734/1761` (zero churn either way).
- Three `// HELIX: External agent …` bypass markers in `extensions_ui.rs` — intact at lines 226, 248, 1518 (unchanged from pre-merge — `f39cf25c0b`'s restructure was confined to its own region around line 1738).

## Validation

- `./stack build-zed dev`: **PASSED** (cargo 16m 59s, total ~18m cold cache, 0 errors, 1 unused-import warning in upstream code).
- Silent-drift sweep: **all clean** (`smol::Timer` 0 hits, `ActiveView`/`set_active_view`/`draft_threads`/`background_threads`/`selected_agent_type` 0 hits, Fix 1b first-statement intact, three `// HELIX:` markers intact, PR #50/#55/#56/#60 surface intact).
- E2E `zed-agent` only: **PASSED** on retry. First run hit a Phase 9 timeout — `zed-agent`'s Anthropic API took ~73s to first-token on the long "Write a detailed explanation of merge sort…" prompt, leaving only 17s for streaming before the 90s phase budget expired. Retry green with Phase 9 explicitly reporting `Received 2 completions -- thread recovered from rapid cancel (correct)`. This is the same class of API-latency flake covered by the documented "one retry permitted" policy (lesson from 001996).
- E2E `zed-agent,claude`: **PASSED** on retry. First run: `zed-agent` ALL 15 phases green; `claude` Phase 1 0-event timeout (claude-agent-acp npx-install bootstrap flake). Second run: `[zed-agent] PASSED`, `[claude] PASSED`, `[store] PASSED` — 28 interactions, 0 interrupted/cancelled, response entries isolation across 8 sessions, thread title sync across 3 sessions.
- Specific phase gates: Phase 9 (PR #60 retry-loop), Phase 15 (PR #55 emit; 82 samples, 407ms longest gap, 22% in final 20%), Phase 16 (0 spontaneous `UserCreatedThread` emits — Fix 1a working) — all explicitly green.

## Test plan

- [x] `./stack build-zed dev` (green, 1 upstream-only warning)
- [x] E2E `zed-agent` (green on retry — API-latency flake on first run)
- [x] E2E `zed-agent,claude` (green on retry — claude npx-install flake on first run)
- [x] Silent-drift sweep (all clean)
- [x] Helix `sandbox-versions.txt` `ZED_COMMIT` bumped to the new merge HEAD (`5ed995947e`) in companion `helixml/helix` PR

Release Notes:

- N/A
