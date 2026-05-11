# Requirements: Merge Latest Zed Upstream (001996)

## Context

Today is **2026-05-11**. The Helix fork of Zed last merged with `zed-industries/zed` via **task 001980** (merge commit `c3e312b056`, integrating upstream `1da60a8518`) on **2026-05-05**. PR #49 landed on fork main as `8b08a3ab1a`.

Since then, **3 Helix-only PRs** landed on fork main (`fe8f4f4e3f`):

| Merge commit | PR | Description |
|---|---|---|
| `fe8f4f4e3f` | #53 | Fix thread detachment when re-opening live session via new sidebar (Critical Fix #11) |
| `910838ae0d` | #52 | Add `cancel_current_turn` command and `turn_cancelled` event (Helix-initiated cancel protocol + E2E phases 13/14) |
| `443af2876d` | #51 | `--headless` flag + synthetic ui_state responder + `E2E_HEADLESS` test mode (already in 001980's branch but PR landed separately) |

Confirmed at run time:
- **Fork HEAD**: `fe8f4f4e3f` (2026-05-08)
- **Upstream HEAD**: `8bdd78e023` "opencode: Update Free models (#56328)" (2026-05-10)
- **Commits to merge**: **127** (3 days of upstream activity)
- **Helix `sandbox-versions.txt` `ZED_COMMIT`**: `fe8f4f4e3f0fb7c0cb51e9c8028ca0c13a8252cb` (matches fork HEAD)

**Risk profile**: medium-high. Commit count (127/3 days) is similar to 001909 (86/3 days), but the diff against the four highest-risk Helix files is unusually large for the time span:

| File | Diff lines | Risk |
|------|------------|------|
| `crates/agent_ui/src/agent_panel.rs` | **1282** | HIGH — heavy upstream churn |
| `crates/acp_thread/src/acp_thread.rs` | 546 | HIGH — overlaps Critical Fixes #6/#8/#9 |
| `crates/agent_ui/src/conversation_view.rs` | 521 | HIGH |
| `crates/agent/src/agent.rs` | 55 | MEDIUM |
| `crates/agent_settings/src/agent_settings.rs` | 6 | LOW |

Specific upstream commit of concern: **`0a52f80824` "acp_thread: Clear `running_turn` when prompt task drops tx (#55562)"** — directly touches the cancel/`Stopped` machinery that Helix has heavy fixes layered on (Critical Fixes #6, #8, #9, plus PR #52's `cancel_current_turn`). Treat as the trickiest single hunk of this merge.

The previous merge spec (`001980_merge-latest-zed/`) is the closest precedent and **must be read in full** before starting. The porting guide entry `## Merge 001980 (2026-05-05)` records the 4 conflicts resolved (workflow YAML, `Cargo.lock`, `agent_settings.rs`, `wgpu_renderer.rs`) and the test-pattern repair pattern (`Stopped(_)`).

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so we stay current and minimise future merge debt — particularly given the heavy upstream churn in `agent_panel.rs` and the new `cancel_current_turn` Helix protocol that overlaps with upstream's `running_turn` clearing fix.

### 2. Helix User
> As a Helix user, I want any new upstream Zed editor improvements without losing the WebSocket sync integration — including the newly added `cancel_current_turn` flow that lets the Helix UI interrupt a running turn.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` updated **as work proceeds** — including a new entry for upstream `0a52f80824` and any silent renames in the heavy `agent_panel.rs` diff — so the rebase checklist reflects current reality.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (or the latest safe point, with any skipped commits explicitly justified in `portingguide.md`)
- [ ] All Helix-specific commits since 001980 (PRs #51–#53) are preserved and functional
- [ ] No upstream commits silently cherry-picked out

### Critical Fix Preservation (the 11 fixes in `portingguide.md` §"Critical Fixes")
- [ ] Fix #1: `NativeAgent` entity cloned before async task in `load_session()`
- [ ] Fix #2: No duplicate WebSocket sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (cf. `cargo test -p acp_thread test_second_send`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal completion path
- [ ] Fix #10: context-server request timeout still 180s (PR #47 — verified in `crates/context_server/src/client.rs`)
- [ ] Fix #11: entity-identity guard at top of `load_agent_thread` in `agent_panel.rs` (PR #53 — sidebar re-open split-brain)

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
- [ ] `--headless` CLI flag still in `crates/zed/src/main.rs` and all 3 call sites intact (per checklist 39a)
- [ ] `rust-embed` workspace dep still has `debug-embed` feature
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] Feature propagation chain `zed → agent_ui → title_bar` intact (`title_bar` dep `optional = true`)

### Newly Added Helix Behaviour Since 001980 (PRs #51–#53 — must survive merge)
- [ ] PR #51 `--headless` flag, `initialize_headless()`, synthetic ui_state responder, `E2E_HEADLESS=1` mode all intact
- [ ] PR #52 `cancel_current_turn` WebSocket command routes through `thread_service` to `AcpThread::cancel()`
- [ ] PR #52 `turn_cancelled` event sent back to Helix with status `cancelled` or `noop`
- [ ] PR #52 protocol tests for `cancel_current_turn` still compile and pass
- [ ] PR #53 entity-identity guard at top of `load_agent_thread` (Critical Fix #11) preserved — clicking the live thread in the sidebar must not detach the panel from the live `Entity<AcpThread>`

### Build & Test (hard gates)
- [ ] `./stack build-zed dev` produces a working binary (`./zed-build/zed`) with zero errors
- [ ] (If local Rust toolchain available) `cargo check -p zed` (no features) — zero errors
- [ ] (If local Rust toolchain available) `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] (If local Rust toolchain available) `cargo test -p external_websocket_sync` — full pass (≤2 env-dependent ignored acceptable)
- [ ] (If local Rust toolchain available) `cargo test -p acp_thread test_second_send` — passes
- [ ] **External WebSocket sync E2E (Docker, hard gate)** — all in-tree phases pass for **both** `zed-agent` and `claude` agents. Contractual minimum:
  - Phase 1: thread A id correct, entry count ≥ 2
  - Phase 2: same thread id, entry count grew
  - Phase 3: thread B id correct, entry count ≥ 2
  - Phase 4: follow-up to non-visible thread completes with no thread-load error
  - Phase 8: mid-stream interrupt — both `send()` calls emit exactly one `Stopped`
  - Phase 9: rapid 3-turn cancel — no deadlock, correct turn ordering
  - **Phase 13** (PR #52): `cancel_current_turn` cancels the active turn, `turn_cancelled` event with `status=cancelled`
  - **Phase 14** (PR #52): `cancel_current_turn` with no active turn returns `status=noop`

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] Each entry records *what changed upstream*, *what the resolution was*, *why*
- [ ] Commit-history table at the bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended with any new fragility uncovered (e.g. silent identifier renames in the large `agent_panel.rs` diff)
- [ ] Stale guide entries discovered along the way are corrected or deleted (do not invent updates)

### Process
- [ ] Feature branch `feature/001996-merge-latest-zed` created from fork main (`fe8f4f4e3f`, or newer if anyone pushed to fork main meanwhile)
- [ ] Branch pushed to `helixml/zed`
- [ ] PR opened against fork main with the merge commit
- [ ] Helix repo PR opened to bump `ZED_COMMIT` in `sandbox-versions.txt` — **opened first per `CLAUDE.md` ordering rule** (Helix PR opens before Zed PR is merged)
- [ ] `main` is **not** force-pushed without explicit user approval

## Out of Scope

- Net-new Helix feature development
- Modifying e2e test assertions themselves (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires
