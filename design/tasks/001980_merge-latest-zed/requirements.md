# Requirements: Merge Latest Zed Upstream (001980)

## Context

Today is **2026-05-05**. The Helix fork of Zed last merged with `zed-industries/zed` via **zed PR #43** (task 001909, merge commit `8428a4399d`, integrating upstream `e3d1876c06` "Revert terminal changes from #54728") on **2026-04-25**.

Two follow-up merge specs (001946 on 2026-04-27, 001947 on 2026-04-27) were planned but **never executed** — no `feature/001946-*` or `feature/001947-*` branch exists on `helixml/zed`. As a result the fork is **~10 days behind upstream**, not 2 days. Their work plans were never wasted; they accurately describe the same fork state and remain useful precedent.

Fork main HEAD is still `f5fab97857`. The four Helix-only PRs since 001909 are unchanged:

| Commit | PR | Description |
|---|---|---|
| `f5fab97857` | #47 | Bump context-server request timeout 60s → 180s |
| `5dea310849` | #46 | Re-route `AgentConnectionStore` through `AgentConnectionCache` |
| `67c87da708` | #45 | Refresh `turn_request_id` on `UserMessage NewEntry` |
| `3c17c13a11` | #44 | Trailing-edge flush timer for streaming throttle |

**Risk profile**: medium. Larger than 001909 (86 commits / 3 days, 1 conflict) but expected to be much smaller than 001864 (920 commits / 30 days, 35 conflicts). Upstream count and HEAD will be confirmed by the implementation agent after `git fetch upstream`.

The previous qwen-code upstream merge spec (`spt_01kp694trspxvkpzp1b9mq7xyh`, branch `feature/001804-we-havent-updated-qwen`) is closest comparable precedent for cross-project merge mechanics, though Zed merge mechanics are fork-specific and best derived from `portingguide.md`.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so we stay current and minimise future merge debt — especially after two skipped windows.

### 2. Helix User
> As a Helix user, I want any new upstream Zed editor improvements without losing the WebSocket sync integration that connects Zed to the Helix platform.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` updated **as work proceeds** — not retrospectively — so that next time the rebase checklist reflects current reality.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (or the latest safe point, with any skipped commits explicitly justified in `portingguide.md`)
- [ ] All Helix-specific commits since 001909 (PRs #44–#47) are preserved and functional
- [ ] No upstream commits silently cherry-picked out

### Critical Fix Preservation (the 9 fixes in `portingguide.md` §"Critical Fixes")
- [ ] Fix #1: `NativeAgent` entity cloned before async task in `load_session()`
- [ ] Fix #2: No duplicate WebSocket sends from `crates/agent_ui/src/acp/thread_view.rs`
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
- [ ] Channel-based UI event forwarding (`tokio::sync::mpsc`)
- [ ] `OnboardingUpsell::set_dismissed` Helix-mode UI cleanup path
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override
- [ ] Built-in agent hiding (Claude Code, Codex, Gemini hidden in Helix builds)
- [ ] Enterprise TLS skip (`sync_settings`)
- [ ] `--allow-multiple-instances` CLI flag still in `crates/zed/src/main.rs`
- [ ] `rust-embed` workspace dep still has `debug-embed` feature
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] Feature propagation chain `zed → agent_ui → title_bar` intact (`title_bar` dep `optional = true`)

### Recently Added Helix Behaviour (PRs #44–#47 — must survive merge)
- [ ] PR #44 trailing-edge flush timer for streaming throttle still in place
- [ ] PR #45 `turn_request_id` refresh on UserMessage `NewEntry` still applied
- [ ] PR #46 `AgentConnectionStore` → `AgentConnectionCache` wiring intact
- [ ] PR #47 context-server request timeout still 180s (not reverted to upstream 60s)

### Build & Test (hard gates)
- [ ] `cargo check -p zed` (no features) — zero errors
- [ ] `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] `./stack build-zed dev` produces a working binary (`./zed-build/zed`)
- [ ] `cargo test -p external_websocket_sync` — 37 pass (≤2 env-dependent ignored acceptable)
- [ ] `cargo test -p acp_thread test_second_send` — passes
- [ ] **External WebSocket sync E2E (Docker, hard gate)** — all in-tree phases pass for **both** `zed-agent` and `claude` agents. The contractual minimum named in this spec:
  - Phase 1: thread A id correct, entry count ≥ 2
  - Phase 2: same thread id, entry count grew
  - Phase 3: thread B id correct, entry count ≥ 2
  - Phase 4: follow-up to non-visible thread completes with no thread-load error
  - Phase 8: mid-stream interrupt — both `send()` calls emit exactly one `Stopped`
  - Phase 9: rapid 3-turn cancel — no deadlock, correct turn ordering

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] Each entry records *what changed upstream*, *what the resolution was*, *why*
- [ ] Commit-history table at the bottom of `portingguide.md` extended with this merge's commits and any follow-up fixes
- [ ] Rebase checklist extended with any new fragility uncovered (e.g. silent identifier renames)
- [ ] Stale guide entries discovered along the way are corrected or deleted (do not invent updates)

### Process
- [ ] Feature branch `feature/001980-merge-latest-zed` created from fork main (`f5fab97857`, or newer if anyone pushed to fork main meanwhile)
- [ ] Branch pushed to `helixml/zed`
- [ ] PR opened against fork main with the merge commit
- [ ] Helix repo PR opened to bump `ZED_COMMIT` in `sandbox-versions.txt` — **opened first per `CLAUDE.md` ordering rule** (Helix PR opens before Zed PR is merged)
- [ ] `main` is **not** force-pushed without explicit user approval

## Out of Scope

- Net-new Helix feature development
- Modifying e2e test assertions themselves (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires
