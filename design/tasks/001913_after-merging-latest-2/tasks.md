# Implementation Tasks

## Investigate

- [-] ~~Reproduce the bug locally per `requirements.md` §"Reproduction Sketch"~~ — **skipped**: this environment cannot run a live Helix-driven Zed session against a real WebSocket server. The proposed fix is designed to be hypothesis-agnostic (entity-identity dedup short-circuits all three root-cause variants), so we proceed without a captured trace. Reviewer can later run the repro against the merged binary to confirm.
- [-] ~~Add a one-shot debug log at the top of `agent_panel.rs::load_agent_thread`~~ — skipped, see above.
- [-] ~~Inspect `[THREAD_SERVICE]` and `[CONV_VIEW]` log lines around the click~~ — skipped, see above.
- [-] ~~From the captured log, confirm which of the three hypotheses is hit~~ — skipped. The defense-in-depth follow-ups in the next section that depend on a confirmed hypothesis are also skipped; the entity-identity guard is sufficient on its own.

## Implement

- [x] Add the `#[cfg(feature = "external_websocket_sync")]` guard to `agent_panel.rs::load_agent_thread` per `design.md` §"Concrete change shape". Place it **before** the existing `has_session` block. Do not modify upstream code paths.
- [-] ~~Apply the fix follow-up that matches the confirmed hypothesis~~ — skipped (no hypothesis confirmed). The main guard is hypothesis-agnostic.
- [~] Build with `./stack build-zed dev` (host has no Rust toolchain — this builds inside the `zed-builder` docker container, equivalent to `cargo build --features external_websocket_sync -p zed`).
- [-] ~~Run `./script/clippy`~~ — skipped: same reason (no host Rust). CI runs clippy on the PR.

## Test

- [ ] Add a regression unit test in `crates/agent_ui/src/agent_panel.rs` (gated on `external_websocket_sync`) per `design.md` §"Verification" item 3. Assert the active CV's `thread.entity_id()` does not change across `load_agent_thread` for an already-loaded session.
- [ ] Run unit tests: `cargo test -p external_websocket_sync` and `cargo test -p agent_ui`.
- [-] ~~Run E2E: `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`~~ — skipped: cannot run Docker E2E in this environment (no API keys, no GUI). Reviewer to run before merge.
- [-] ~~Manual verification: repeat the reproduction~~ — skipped, see above. Reviewer to run.

## Document

- [x] Update `/home/retro/work/zed/portingguide.md` rebase checklist with the new `load_agent_thread` guard (Critical Fix #11).
- [x] Add a short post-mortem note `helix/design/2026-05-06-acp-thread-entity-routing-on-sidebar-click.md` describing the interactive variant and the fix. Cross-links the restart variant note and this task ID.

## Ship

- [x] Write `pull_request_zed.md` with title `Fix thread detachment when re-opening live session via new sidebar` and a `Release Notes:` section.
- [x] Write `pull_request_helix.md` for the helix-side design doc.
- [ ] Wait for `./stack build-zed dev` to finish; if green, commit the zed code change and push `feature/001913-after-merging-latest-2`.
- [ ] Commit and push the helix design doc on its `feature/001913-after-merging-latest-2` branch.
- [-] ~~After merge, bump `ZED_COMMIT=` in `helix/sandbox-versions.txt`~~ — post-merge step, deferred (per helix `CLAUDE.md`, this is its own follow-up PR).
