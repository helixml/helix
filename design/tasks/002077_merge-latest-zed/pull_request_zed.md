# Merge latest upstream Zed (256 commits, 2026-06-02 → 2026-06-12)

## Summary

Brings the Helix fork of Zed up to date with `zed-industries/zed` HEAD (`992f395c3d`), absorbing 256 upstream commits over 10 days. **Clean merge** — only 6 trivial conflicts (5 workflow/deletion + 1 import-block), zero Helix-side signature-drift repairs needed, build green on first attempt, full E2E green for both `zed-agent` and `claude` personalities.

This is the first upstream merge since 002029 that required **zero** "Pre-existing Breakage Repaired" entries.

## Changes

- `Merge remote-tracking branch 'upstream/main'` (`b2993c0b01`) — 256 upstream commits including the compaction cluster (built-in `/compact` slash command, `auto_compact` setting), `d7ac5e6cf4` "Preserve waiting tool call status on updates" (+602 lines), `215ca2fb0b` "Typed workspace errors", `116e4bc184` "Inherit source agent without draft content", `27191913e9` "Cumulative token usage", `56b71271c4` "Enable ACP session usage and deletion features", `620ceaaaca` "Flush thread content to database on app quit", and `a98485809b` "Return typed completion errors from Cloud provider" (which deleted `crates/language_model/src/model/` entirely).
- `docs(porting): add Merge 002077 entry (256 upstream commits, 0 breakage)` (`38d4f86809`) — new `## Merge 002077 (2026-06-12)` section in `portingguide.md` documenting all 6 conflicts, the auto-merge survival check, and the risks that turned out to be non-issues.

### Conflicts resolved

1. `.github/workflows/run_cron_unit_evals.yml`, `run_unit_evals.yml` — `git rm` (upstream deletion; Helix doesn't use Zed's CI).
2. `.github/workflows/slack_notify_first_responders.yml` — `--theirs`.
3. `crates/language_model/src/model/mod.rs` — accept upstream deletion (entire `model/` directory removed; `CloudModel` relocated to `language_model_core` + `language_models/src/provider/cloud.rs`). No Helix surface referenced it.
4. `crates/recent_projects/src/dev_container_suggest.rs` — kept Helix's `use settings::Settings;` (Helix `suggest_dev_container` guard) + upstream's new `use std::path::Path;` (new parameter type).
5. `crates/title_bar/src/title_bar.rs` — import block: kept Helix's `cloud_api_types::Plan` + cfg-gated `external_websocket_sync::{...}` AND added upstream's new `command_palette_hooks::CommandPaletteFilter`.

### Helix surface — auto-merge survival check

All Helix critical fixes and load-bearing patches survived the merge cleanly with **no manual conflict in any Helix-touched source file** (`agent.rs`, `acp_thread.rs`, `agent_panel.rs`, `conversation_view.rs`, `workspace.rs`, `agent_servers/src/acp.rs`, `zed/src/main.rs`, `extensions_ui.rs`, `feature_flags/src/flags.rs`):

- Critical Fixes #1, #3, #6, #8, #9, #11 — intact.
- PR #50 `session_creation_chain` + `_settings_subscription` — intact.
- PR #55 streaming-reveal `EntryUpdated` emit — intact (survived `d7ac5e6cf4`'s 602-line tool-call-status rewrite).
- PR #56 Fix 1a deferred `UserCreatedThread` — intact.
- PR #56 Fix 1b cfg-gated early return — intact and verified as the FIRST statement of `BaseView::Uninitialized`.
- PR #60 `ede_diagnostic` retry loop — intact (no upstream churn in `thread_service.rs` this window).

### Planning-time risks that turned out to be non-issues

- `d7ac5e6cf4` "Preserve waiting tool call status" — auto-merged; PR #55 emit site survived.
- Compaction cluster (~1700 net lines) — auto-merged; no WS payload schema changes (`grep "compact\|cumulative_token_usage" crates/external_websocket_sync/` returns nothing).
- `215ca2fb0b` typed workspace errors — auto-merged; Helix surface has **zero** `Workspace::show_error` call sites, so the typed-error migration is not needed.
- `116e4bc184` source-agent inheritance — auto-merged; Fix 1b's first-statement position survives.
- `620ceaaaca` flush-on-quit — auto-merged; no interaction with WS-authoritative store.
- `56b71271c4` ACP session usage/deletion stabilisation — auto-merged; no Helix `AcpConnection` override needed.

## Validation

- `./stack build-zed dev`: green (8m 14s, 0 errors, 3 unused-import warnings — all in upstream code).
- Silent-drift sweep (`ActiveView` / `set_active_view` / `draft_threads` / `background_threads` / `selected_agent_type` / `smol::Timer` / non-tuple `Stopped` / `Workspace::show_error` in Helix surface / `cumulative_token_usage`-in-WS / `compact`-in-WS): all zero hits or only expected matches.
- E2E `zed-agent` only: **PASSED** (full phase suite; store validation 14 interactions / 0 interrupted/cancelled / 4 sessions / response entries isolation PASSED / thread title sync PASSED).
- E2E `zed-agent,claude`: **PASSED** (both personalities green; store validation PASSED).

## Test plan

- [x] `./stack build-zed dev` (green)
- [x] E2E `zed-agent` (green)
- [x] E2E `zed-agent,claude` (green — both personalities)
- [x] Silent-drift sweep (all clean)
- [x] Helix `sandbox-versions.txt` `ZED_COMMIT` bumped to the new merge HEAD (`38d4f86809`) in companion `helixml/helix` PR

Release Notes:

- N/A
