# Requirements: Merge Latest Zed Upstream

## Context

The Helix fork of Zed was last merged with upstream `zed-industries/zed` via task **001909** (merge commit `8428a4399d`, integrating upstream `e3d1876c06` "Revert terminal changes from #54728") and PR #43, on **April 25, 2026**. Since then four more Helix-specific commits have landed on fork main:

- `3c17c13a11` — trailing-edge flush timer for streaming throttle (PR #44, task 001895)
- `67c87da708` — refresh `turn_request_id` on `UserMessage NewEntry` (PR #45)
- `5dea310849` — re-route `AgentConnectionStore` through `AgentConnectionCache` (PR #46)
- `6c66a0fdc7` — bump context-server timeout 60s → 180s (PR #47)

Current fork HEAD: `f5fab97857`. Today is **April 27, 2026** — only ~2 days of upstream activity have accumulated. This is expected to be the **smallest merge yet** (smaller than 001909's 86-commit / 3-day window). Upstream commit count and HEAD will be confirmed by the implementation agent after `git fetch upstream`.

Risk profile: low. But the same care must be taken as on every prior merge — the fragile cfg-gated regions in `agent_panel.rs`, `conversation_view.rs`, `acp_thread.rs`, and `agent.rs` remain the dominant risk.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so that we stay current and minimise future merge debt.

### 2. Helix User
> As a Helix user, I want any new upstream Zed editor improvements without losing the WebSocket sync integration that connects Zed to the Helix platform.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` kept current with any new patterns, renames, trait changes, or fragile spots discovered during this merge — written **as work proceeds**, not retrospectively.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD
- [ ] All Helix-specific commits since the last merge are preserved and functional
- [ ] No upstream commits skipped or cherry-picked out

### Critical Fix Preservation (the 9 fixes in `portingguide.md` §"Critical Fixes")
- [ ] Fix #1: `NativeAgent` entity cloned before async task in `load_session()`
- [ ] Fix #2: No duplicate WebSocket sends from `thread_view.rs` (only `UserCreatedThread` + `ThreadTitleChanged`)
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (cf. `cargo test -p acp_thread test_second_send`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal completion path

### Helix-Specific Surface (must survive verbatim or be re-applied with equivalent semantics)
- [ ] `crates/external_websocket_sync/` crate intact
- [ ] WebSocket thread display callback in `agent_panel.rs::new()`
- [ ] UI state query callback in `agent_panel.rs::new()`
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] Session persistence methods (load/restore in panel restoration path)
- [ ] `from_existing_thread()` constructor on `ConversationView`
- [ ] Channel-based UI event forwarding (`tokio::sync::mpsc` instead of GPUI subscriptions)
- [ ] `OnboardingUpsell::set_dismissed` path (Helix-mode UI cleanup)
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override
- [ ] Built-in agent hiding (Claude Code, Codex, Gemini hidden in Helix builds)
- [ ] Enterprise TLS skip (sync_settings)
- [ ] `--allow-multiple-instances` CLI flag still present
- [ ] `rust-embed`'s `debug-embed` workspace feature still set
- [ ] Feature propagation chain `zed → agent_ui → title_bar` still intact

### Build & Test (hard gates)
- [ ] `cargo check -p zed` (no features) — zero errors
- [ ] `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] `cargo test -p external_websocket_sync` — 37 pass, ≤2 env-dependent ignored acceptable
- [ ] `./stack build-zed dev` produces a working binary
- [ ] **E2E Docker test for `external_websocket_sync` — all four phases pass** (Phase 1: thread A id + entries ≥ 2; Phase 2: same id, entries grow; Phase 3: thread B id, entries ≥ 2; Phase 4: follow-up to non-visible thread completes with no thread-load error). The current test in tree has 12 phases — **all** in-tree phases must pass; the four phases above are the contractual minimum named in this spec.

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] Each resolution entry records: *what changed upstream*, *what the resolution was*, *why*
- [ ] Commit history table extended with this merge's commits and any follow-up fixes
- [ ] Rebase checklist appended with any new fragility discovered

### Process
- [ ] Feature branch `feature/001947-merge-latest-zed` created from fork main
- [ ] Branch pushed to `helixml/zed`
- [ ] PR opened against fork main with merge commit
- [ ] Helix repo PR opened to bump `ZED_COMMIT` in `sandbox-versions.txt` (per `CLAUDE.md` ordering: Helix PR opened *before* Zed PR is merged)
- [ ] `main` is **not** force-pushed without explicit user approval
