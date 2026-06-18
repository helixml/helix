# Merge latest upstream Zed (120 commits across two rounds, 2026-06-12 → 2026-06-18)

## Summary

Brings the Helix fork of Zed up to date with `zed-industries/zed` HEAD (`e45e42af6e`), absorbing **120 upstream commits over 6 days** across two merge rounds (round 1: 25 commits / `a31d3505da`; round 2 / extension: 95 commits / `e45e42af6e`). **Two trivial conflicts total** — one per round, each a localized signature-drift collision (not a deep state-machine clash). Zero Helix-side signature-drift repairs needed, build green on first attempt each round, full E2E green for both `zed-agent` and `claude` personalities.

This is the **third consecutive** upstream merge that required **zero** "Pre-existing Breakage Repaired" entries (002077 was the first, 002100 round 1 the second).

## Changes

- `Merge remote-tracking branch 'upstream/main'` (`0098823efa`) — 25 upstream commits including: editor bracket-splitting in line comments (`cccc7b2d44`), `objc2-app-kit` 0.3 → 0.3.2 with feature widening, GPUI macOS traffic-light hitbox fix (`138139f830`), audio phantom-presence in channels fix (`e5b6041e9d`), `multi_buffer` `range_to_buffer_ranges` micro-optimisation (`b6c7496aea`), repl notebook-cell kernel error display (`26a355b11d`), `git stash list` per-file-save optimisation (`a31d3505da`), `dev_container` BuildKit toggle (`26fc42721a`) and `$VAR` expansion (`832ab56db8`), agent shell-hang fix on syntax errors (`c578f4d12b`), extension_ui chip-filter restructure to hide agent servers (`f39cf25c0b`), and a `[patch.crates-io] async-process` entry to allow reaper reuse (`d4cc8d2409`).
- `docs(porting): add Merge 002100 entry` (`952f59f2d6`) — new `## Merge 002100 (2026-06-15)` section in `portingguide.md` documenting the single conflict, the auto-merge survival check across all critical fixes / PR #50 / PR #55 / PR #56 / PR #60 surface, and the risks that were considered in planning and turned out to be non-issues.
- `docs(porting): record 002100 build+E2E results` (`5ed995947e`) — fills in the Validation section after build + E2E completed green.

### Conflicts resolved (one per round)

**Round 1** (`0098823efa`):
1. `crates/settings_content/src/settings_content.rs` — **both sides added a field on `RemoteSettingsContent`**. Helix's `suggest_dev_container: Option<bool>` (drives the `dev_container_suggest.rs` early-return guard) and upstream's `dev_container_use_buildkit: Option<bool>` from `26fc42721a` "dev_container: Support the classic Docker builder via a setting (#59288)". Kept both — independent settings, no semantic overlap.

**Round 2 / extension** (`0e0149ade5`):
2. `crates/agent/src/tools/grep_tool.rs` — upstream `40211567b8` "Make grep tool results clickable in agent panel (#59230)" refactored the snippet handling: introduced `let snippet: String = snapshot.text_for_range(range.clone()).collect();` and reused `snippet` both for the markdown block and for a new clickable `acp::ToolCallContent::Content(ContentBlock::Text(...))` emit. Helix had a `truncate_long_lines(&text, MAX_LINE_CHARS)` wrapper around the markdown block (from task 001410, prevents context-window blowups). Resolution: kept the truncation, dropped the now-redundant `text` binding, used upstream's pre-computed `snippet` instead → `output.push_str(&truncate_long_lines(&snippet, MAX_LINE_CHARS));`.

All other files auto-merged cleanly across both rounds. The round 2 auto-merge handled +198 lines in `acp_thread.rs`, +223 in `agent.rs`, +511 in `agent/src/thread.rs`, +1024 in `conversation_view.rs`, a new `thread_search_bar.rs` (+962), +1094 in `thread_view.rs`, +458 in `agent/src/sandboxing.rs`, +957 in `agent/src/tools/terminal_tool.rs`, +286 in `extensions_ui.rs`, +203 in `agent_panel.rs`, +36 in `title_bar.rs` — all without manual intervention.

### Helix surface — auto-merge survival check

Round 1 had zero upstream churn in `acp_thread/`, `agent/src/`, `workspace.rs`, etc., so the must-survive surface was untouched by construction. **Round 2 touched all of them heavily** and the surface still survived — verified intact across both rounds:

- Critical Fix #1 (`load_session` via `pending_sessions`): intact at `agent/src/agent.rs:399/572/1612`.
- Critical Fix #3 (`content_only`): intact at `acp_thread.rs:335` (shifted from 262 — upstream added new content above it).
- Critical Fix #6/#9 (`stopped_emitted_for_task`): intact at `acp_thread.rs:2887/2931/3026`.
- Critical Fix #8 (`drop(turn.send_task)`): intact at `acp_thread.rs:3079`.
- Critical Fix #11 (entity-identity guard): intact at top of `load_agent_thread` in `agent_panel.rs`.
- PR #50 `session_creation_chain` + `_settings_subscription`: intact at `agent_servers/src/acp.rs:438-439`.
- PR #55 streaming-reveal `EntryUpdated` emit: intact (16 occurrences in `acp_thread.rs`).
- PR #56 Fix 1a deferred `UserCreatedThread`: intact in `external_websocket_sync/src/thread_service.rs`.
- PR #56 Fix 1b cfg-gated early return: intact and verified as the FIRST statement of `BaseView::Uninitialized` at `agent_panel.rs:5468-5473` (shifted from 5420 — upstream added +49 lines of compaction/notification handling above; content preserved verbatim).
- PR #60 `ede_diagnostic` retry loop: intact at `thread_service.rs:1916/1976`.
- PR #63 claude-agent-acp wedge recovery (force-reset, keep-alive clear, agent_name-per-thread tracking): intact (no upstream churn in this Helix-only territory).
- PR #64 `agent_ready` re-emit on reopen: intact (Helix-only).
- Three `// HELIX: External agent …` bypass markers in `extensions_ui.rs`: intact at lines 337, 359, 1629 (shifted from 226, 248, 1518 — upstream added new chips above them).
- `AcpBetaFeatureFlag::enabled_for_all() -> true`: intact at `feature_flags/src/flags.rs:30` (despite +16 lines around it).
- `render_restricted_mode` cfg-gated early return in `title_bar.rs`: intact at line 699 (was 678).
- `build_application(headless: bool)` + `--allow-multiple-instances` + `--headless` in `zed/src/main.rs`: intact at lines 88/346.

## Validation

### Round 1 (`5ed995947e`)
- `./stack build-zed dev`: PASSED (cargo 16m 59s cold cache, 0 errors, 1 upstream-only warning).
- E2E `zed-agent` only: PASSED on retry (first run: Phase 9 API-latency flake; retry green).
- E2E `zed-agent,claude`: PASSED on retry (first run: claude Phase 1 npx-install flake; retry green — 28 interactions, 0 interrupted/cancelled).

### Round 2 / extension (`7e0a439153`)
- `./stack build-zed dev`: PASSED (cargo 3m 37s warm cache, 0 errors, 2 upstream-only warnings).
- Silent-drift sweep: all clean — Fix 1b first-statement intact at new line 5468, three `// HELIX:` markers intact at new lines 337/359/1629, PR #50/#55/#56/#60/#63/#64 surface intact.
- E2E `zed-agent` only: **PASSED on first try** — all 15 phases green, store validation PASSED, 14 interactions / 0 interrupted/cancelled.
- E2E `zed-agent,claude`: PASSED on retry. First attempt was `--no-build` against a cached test-server binary and produced a `response_entries leaked across interactions` isolation violation. Full rebuild (no `--no-build`) produced clean results: `[zed-agent] PASSED`, `[claude] PASSED`, `[store] PASSED`, 28 interactions / 0 interrupted/cancelled / response-entries isolation across 8 sessions / thread-title sync across 3 sessions.
- Specific phase gates: Phase 9 (PR #60 retry-loop), Phase 15 (PR #55 emit), Phase 16 (0 spontaneous `UserCreatedThread` emits — Fix 1a working) — all explicitly green.

## Test plan

- [x] `./stack build-zed dev` (green both rounds)
- [x] E2E `zed-agent` (green; round 2 was first-try green)
- [x] E2E `zed-agent,claude` (green; both rounds required one retry — round 1 due to claude Phase 1 npx-install flake, round 2 due to using `--no-build` against a stale test-server binary)
- [x] Silent-drift sweep (all clean both rounds)
- [x] Helix `sandbox-versions.txt` `ZED_COMMIT` bumped to the new merge HEAD (`7e0a4391535f07ad8413f5a3bc8c318775eaacee`) in companion `helixml/helix` PR

### Suggested .rules additions

- **`--no-build` is a footgun for the E2E test server.** When the Helix Go code (`api/`) has advanced relative to the cached test-server binary, the `--no-build` shortcut runs an out-of-date server against current Zed and can produce false positive `response_entries leaked across interactions` store-validation failures. The e2e-test CLAUDE.md already warns about this; consider promoting it to the main `crates/external_websocket_sync/.rules` so it surfaces in every session.

Release Notes:

- N/A
