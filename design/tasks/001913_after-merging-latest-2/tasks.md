# Implementation Tasks

## Investigate

- [ ] Reproduce the bug locally per `requirements.md` §"Reproduction Sketch" — **single-thread, single-click is the canonical case**. Capture `Zed.log` and Helix server logs from one before/after pair.
- [ ] Add a one-shot debug log at the top of `agent_panel.rs::load_agent_thread` printing: the incoming `session_id`, the current `BaseView` discriminant, and (if `BaseView::AgentThread`) the active `cv.root_session_id` and the active `cv.active_thread().map(|t| t.thread.entity_id())`. Include also the result of `external_websocket_sync::get_thread(session_id_str)`'s `entity_id()`. This single log line is enough to disambiguate all three hypotheses in one repro.
- [ ] Inspect `[THREAD_SERVICE]` and `[CONV_VIEW]` log lines around the click. Specifically look for `register_thread: overwriting thread '…' with different entity` (confirms split-brain) and `Creating NEW subscription for thread … on entity` (confirms a second subscription was created).
- [ ] From the captured log, confirm which of the three hypotheses in `design.md` §"Root-Cause Hypothesis" is hit: (1) `root_session_id` is `None`/stale on the active CV, (2) `metadata.session_id` ≠ entity's `session_id()`, (3) `BaseView` is `Uninitialized`/draft when the click arrives. Note the result inline in the PR description. If hypothesis #1 is confirmed, find the assignment that's responsible and fix it as well (so non-Helix builds aren't silently broken).

## Implement

- [ ] Add the `#[cfg(feature = "external_websocket_sync")]` guard to `agent_panel.rs::load_agent_thread` per `design.md` §"Concrete change shape". Place it **before** the existing `has_session` block. Do not modify upstream code paths.
- [ ] Apply the fix follow-up that matches the confirmed hypothesis: (#1) repair the `root_session_id` assignment so it stays consistent across `set_server_state` / `reset` / reconnect; (#2) align `root_session_id` to use the entity's `session_id()` in `ConversationView::new` (`crates/agent_ui/src/conversation_view.rs` ~line 741); (#3) ensure `notify_thread_display` runs (or the panel restoration produces a `BaseView::AgentThread` for the loaded thread) before the user can click. Skip whichever don't apply.
- [ ] Build with `cargo build --features external_websocket_sync -p zed` and with the default features. Both must succeed.
- [ ] Run `./script/clippy` (per `crates/zed/CLAUDE.md`).

## Test

- [ ] Add a regression unit test in `crates/agent_ui/src/agent_panel.rs` (gated on `external_websocket_sync`) per `design.md` §"Verification" item 3. Assert the active CV's `thread.entity_id()` does not change across `load_agent_thread` for an already-loaded session.
- [ ] Run unit tests: `cargo test -p external_websocket_sync` and `cargo test -p agent_ui`.
- [ ] Run E2E: `docker build -t zed-ws-e2e -f crates/external_websocket_sync/e2e-test/Dockerfile . && docker run --rm -e ANTHROPIC_API_KEY=… -e E2E_AGENTS=zed-agent,claude -e TEST_TIMEOUT=180 zed-ws-e2e`. Both agents must complete all phases.
- [ ] Manual verification: repeat the reproduction. Active CV's entity ID stable across the click; Zed panel keeps streaming; no `overwriting thread` warning in logs.

## Document

- [ ] Update `/home/retro/work/zed/portingguide.md` rebase checklist with a one-line note about the new `load_agent_thread` guard (per `design.md` §"Documentation").
- [ ] Add a short post-mortem note to `helix/design/2026-04-24-acp-thread-entity-routing-after-restart.md` (or a sibling file) describing the interactive variant and the fix. Cross-link this task ID.

## Ship

- [ ] Open a PR against `helixml/zed` with title `Fix thread detachment when re-opening live session via new sidebar` (no conventional-commit prefix, no trailing punctuation per `crates/zed/CLAUDE.md`).
- [ ] PR body includes a `Release Notes:` section with `- Fixed thread becoming detached when clicking the currently-open thread in the new agents sidebar (Helix mode).`.
- [ ] After merge, bump `ZED_COMMIT=` in `/home/retro/work/helix/sandbox-versions.txt`, open a Helix PR, and let the build pipeline pick it up.
