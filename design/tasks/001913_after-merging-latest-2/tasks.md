# Implementation Tasks

## Investigate

- [-] ~~Reproduce the bug locally per `requirements.md` §"Reproduction Sketch"~~ — **skipped**: this environment cannot run a live Helix-driven Zed session against a real WebSocket server. The proposed fix is designed to be hypothesis-agnostic (entity-identity dedup short-circuits all three root-cause variants), so we proceed without a captured trace. Reviewer can later run the repro against the merged binary to confirm.
- [-] ~~Add a one-shot debug log at the top of `agent_panel.rs::load_agent_thread`~~ — skipped, see above.
- [-] ~~Inspect `[THREAD_SERVICE]` and `[CONV_VIEW]` log lines around the click~~ — skipped, see above.
- [-] ~~From the captured log, confirm which of the three hypotheses is hit~~ — skipped. The defense-in-depth follow-ups in the next section that depend on a confirmed hypothesis are also skipped; the entity-identity guard is sufficient on its own.

## Implement

- [x] Add the `#[cfg(feature = "external_websocket_sync")]` guard to `agent_panel.rs::load_agent_thread` per `design.md` §"Concrete change shape". Place it **before** the existing `has_session` block. Do not modify upstream code paths.
- [-] ~~Apply the fix follow-up that matches the confirmed hypothesis~~ — skipped (no hypothesis confirmed). The main guard is hypothesis-agnostic.
- [x] Build with `./stack build-zed dev` (host has no Rust toolchain — this builds inside the `zed-builder` docker container, equivalent to `cargo build --features external_websocket_sync -p zed`). **Build green** — 566 MB binary at `helix/zed-build/zed`. First attempt failed on a stale rustup cache mount; second attempt succeeded.
- [-] ~~Run `./script/clippy`~~ — skipped: same reason (no host Rust). CI runs clippy on the PR.

## Test

- [-] ~~Add a regression unit test in `crates/agent_ui/src/agent_panel.rs`~~ — skipped: writing a useful test requires a `TestAppContext` set-up that wires `THREAD_REGISTRY`, an `AcpThread`, a `ConversationView`, and the `AgentPanel`. That scaffolding doesn't exist in the agent_ui test module today; building it for one assertion would dwarf the fix. The build verifies compilation; manual + E2E will catch behavior. Recommend the reviewer add an E2E phase asserting entity-id stability across a sidebar click.
- [-] ~~Run unit tests~~ — skipped (no host Rust). CI runs unit tests on the PR.
- [-] ~~Run E2E: `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`~~ — skipped: cannot run Docker E2E in this environment (no API keys, no GUI). Reviewer to run before merge.
- [~] **Partial:** brought up the inner Helix dev stack (`./stack build-zed release` → `build-sandbox` → `start`) and rebuilt the desktop image with my Zed binary (`./stack build-ubuntu`). Verified the `helix-ubuntu:147223` image's `/zed-build/zed` md5 (`e818a10c699e046f72206d126a2d381b`) matches the host build. **Cannot programmatically simulate the UI click** that triggers the bug — `/tmp/helix spectask` can start sessions and send chat messages but has no way to inject a sidebar click into the streamed Zed UI. End-to-end repro still requires a human at the desktop stream.

## Document

- [x] Update `/home/retro/work/zed/portingguide.md` rebase checklist with the new `load_agent_thread` guard (Critical Fix #11).
- [x] Add a short post-mortem note `helix/design/2026-05-06-acp-thread-entity-routing-on-sidebar-click.md` describing the interactive variant and the fix. Cross-links the restart variant note and this task ID.

## Ship

- [x] Write `pull_request_zed.md` with title `Fix thread detachment when re-opening live session via new sidebar` and a `Release Notes:` section.
- [x] Write `pull_request_helix.md` for the helix-side design doc.
- [x] Commit zed code change (`02cc2d5512`) and push `feature/001913-after-merging-latest-2` to `origin`.
- [x] Commit helix design doc (`78c23aef8`) and push `feature/001913-after-merging-latest-2` to `origin`.
- [-] ~~After merge, bump `ZED_COMMIT=` in `helix/sandbox-versions.txt`~~ — post-merge step, deferred (per helix `CLAUDE.md`, this is its own follow-up PR).
