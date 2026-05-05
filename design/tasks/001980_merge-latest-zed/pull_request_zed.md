# Merge upstream Zed (172 commits) — task 001980

## Summary

Brings the Helix fork up to date with `zed-industries/zed` HEAD `1da60a8518` ("editor: Extract Diagnostics code out of `editor.rs`"). 172 upstream commits over 10 days, plus 4 carry-overs needed locally to keep the WebSocket sync layer compiling and the test suite green.

## Conflicts and resolutions

| File | Resolution |
|---|---|
| `.github/workflows/deploy_cloudflare.yml` | accept upstream deletion (Helix doesn't use Zed's CI) |
| `Cargo.lock` | `--theirs` (regenerated on next build) |
| `crates/agent_settings/src/agent_settings.rs` | kept Helix's `show_onboarding`/`auto_open_panel` fields; dropped `new_thread_location` to match upstream removal in #55575 |
| `crates/gpui_wgpu/src/wgpu_renderer.rs` | accept upstream comment addition (no Helix concern) |

Per-conflict context, rationale, and risk are recorded inline in `portingguide.md` §"Merge 001980" — written **as each conflict was resolved**, not retrospectively.

## Carry-over fixes

- **`acp_thread.rs:5357,5429`**: `matches!(event, AcpThreadEvent::Stopped)` → `Stopped(_)`. The Helix-added `test_second_send_during_active_turn_emits_stopped_for_both_turns` (Critical Fix #6 verification) and `test_dropped_send_task_clears_running_turn` were silently broken since `Stopped` became a tuple variant in 001864 — never noticed because production builds skip `#[cfg(test)]`. Added new rebase-checklist item 41a so the next merger checks for this trap.
- **`crates/external_websocket_sync/e2e-test/helix-ws-test-server/go.mod` + `go.sum`**: `go mod tidy` regen because helix Go deps had drifted (`kodit v1.3.6 → v1.3.7`, dropped `go-tika`). The e2e runner doesn't tidy itself.

## Verification

- `./stack build-zed dev` — clean (warnings only, 6m 35s)
- All 9 critical fixes verified by source inspection
- All numbered items in `portingguide.md` §"Rebase Checklist" walked
- Silent-drift sweep clean (`ActiveView`, `set_active_view`, `draft_threads`, `selected_agent_type`, `smol::Timer` all 0)
- 001909 carry-overs intact (`--allow-multiple-instances`, `debug-embed`, `cx.background_executor().timer()`)

### E2E (the hard gate)

Both rounds passed end-to-end against a real Anthropic API:

| Agent | Phases | Sync events | Threads |
|---|---|---|---|
| `zed-agent` | **12/12 PASSED** | 213 | 3 |
| `claude` (Claude Code) | **12/12 PASSED** | 168 | 3 |

Phase 1 took 15.1s for `wait_for_tools_ready`, confirming the `cx.background_executor().timer()` fix from 001909 still works. Phase 8 ordering correct, Phase 9 recovered from rapid cancel, Phase 12 reconnect succeeded.

## Helix-Specific Surface (preserved)

`external_websocket_sync` crate intact, `agent_panel.rs` callbacks/accessors intact, `from_existing_thread()` intact (6-field `ConnectedServerState` unchanged), `AcpBetaFeatureFlag::enabled_for_all() -> true`, built-in agent hiding, enterprise TLS skip, `--allow-multiple-instances`, `debug-embed`, feature propagation chain.

PRs #44–#47 all baked into the base — no regressions.

## Pairing

The companion Helix repo PR bumps `ZED_COMMIT` to `42b8107379`.

Release Notes:

- N/A
