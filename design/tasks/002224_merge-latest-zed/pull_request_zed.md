# Merge latest zed-industries/zed upstream into helix-fork (002224)

## Summary

Merges **289 upstream commits** (`e45e42af6e..872ca8fef5`, the 2026-06-18 → 2026-07-06 window) into the Helix fork, absorbing the **ACP `agent-client-protocol` 0.14.0 → 1.0.1 major bump** and keeping all Helix-specific surface behind `#[cfg(feature = "external_websocket_sync")]`. All conflicts resolved following established patterns; `portingguide.md` extended with a full `## Merge 002224 (2026-07-06)` entry written incrementally as each conflict was resolved.

Merge base `e45e42af6e` confirmed; fork was at `9546054e68` (PR #65) with zero Helix-only debt. Also picked up the out-of-band PR #66 (task 002228 prompt-queue e2e phases) via a clean `origin/main` re-merge.

## Conflicts resolved (5 content + 2 workflow modify/deletes)

- `reqwest_client.rs` — kept Helix insecure-TLS branch **and** upstream's new HTTP/2 keep-alive tuning.
- `agent_ui/src/config_options.rs` — kept Helix `current_model_value()`, adapted it to upstream's renamed `first_config_option_id_matching`.
- `agent_panel.rs` — kept Helix imports (deduped post-build).
- `agent_servers/src/acp.rs` — kept PR #50 `SessionCreationGuard` + slot chain alongside upstream's new `client_capabilities_for_agent`; switched `new_session` send to upstream's `.block_task()`.
- `acp_thread.rs` (×2) — preserved Critical Fix #6/#8/#9 guarded `Stopped` emission, added upstream's `StatusChanged` on same-turn.
- `.github/workflows/{hotfix-review-monitor,stale-pr-reminder}.yml` — kept Helix's deletion.

## ACP 1.0 migration repairs (surfaced by the feature build)

1. `into_foreground_future` removed upstream → `.block_task()` on the close-session path.
2. ACP schema moved under `v1` → `thread_service.rs` alias changed to `schema::v1 as acp`.
3. Deduped `agent_panel.rs` imports upstream also added.
4. Removed obsolete `clear_overlay_state()` call (overlay/config-panel system deleted upstream).
5. Added new elicitation fields (`request_elicitation_form_states`, `_request_elicitation_subscription`) to `from_existing_thread`.
6. Fully-qualified `agent::ThreadStore` in cfg-gated headless init.

## Preserved (verified by grep + build)

All 10 active Critical Fixes; PR #50/#55/#56(1a+1b)/#57/#60/#63/#64/#65 surface; `AcpBetaFeatureFlag::enabled_for_all()`; `--headless`/`--allow-multiple-instances`/`initialize_headless`; `rust-embed debug-embed`; `render_restricted_mode` cfg-gate; 3× `// HELIX:` markers. Full per-item audit in `portingguide.md`.

## Testing

- **Build**: `./stack build-zed dev --features external_websocket_sync` — green (580 MB binary).
- **E2E `zed-agent`**: **all 17 phases + store validations PASSED** (run twice), covering the ACP 1.0 fixes, PR #65 `chat_response_error`, cancel/Stopped invariants, streaming, and the queue interrupt path.
- **E2E `claude`**: phases 1–16 pass; Phase 17 (queue interrupt redelivery) does not complete locally. This is **not a regression** — Phase 17 is new from PR #66 and was only ever validated for `zed-agent`; the failure is in `claude-agent-acp` post-cancel redelivery, amplified by a local model substitution (the local proxy 404s the intended `claude-sonnet-5`; ran with `claude-opus-4-8`, reverted). CI (real Anthropic model) is the authoritative `claude` gate.

Release Notes:

- N/A
