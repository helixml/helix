# Implementation Tasks

## Investigate

- [ ] Reproduce the bug locally per `requirements.md` §"Reproduction Sketch". Capture `Zed.log` and Helix server logs from one before/after pair.
- [ ] Inspect `[THREAD_SERVICE]` and `[CONV_VIEW]` log lines around the click. Specifically look for `register_thread: overwriting thread '…' with different entity` (confirms split-brain) and `Creating NEW subscription for thread … on entity` (confirms a second subscription was created).
- [ ] Confirm which of the three hypotheses in `design.md` §"Root-Cause Hypothesis" is hit: (1) ThreadSwitcher preview side-effect, (2) `root_session_id` mismatch between `from_existing_thread` and `external_thread`, (3) stale retained CV. Note the result inline in the PR description.

## Implement

- [ ] Add the `#[cfg(feature = "external_websocket_sync")]` guard to `agent_panel.rs::load_agent_thread` per `design.md` §"Concrete change shape". Place it **before** the existing `has_session` block. Do not modify upstream code paths.
- [ ] If investigation confirmed hypothesis #2, also align `root_session_id` to use the entity's `session_id()` in `ConversationView::new` (`crates/agent_ui/src/conversation_view.rs` ~line 741). Otherwise skip.
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
