# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork

## Phase 0 — Read before touching anything

- [ ] Read `helix-specs/design/tasks/002353_merge-latest-zed/{requirements,design,tasks}.md` in full (the prior cycle's playbook)
- [ ] Read `/home/retro/work/zed/portingguide.md` §"Critical Fixes" (line ~242) and §"Rebase Checklist" (line ~508)
- [ ] Read this task's `requirements.md` and `design.md`

## Phase 1 — Re-measure the window

- [ ] Verify `upstream` remote exists in `/home/retro/work/zed` (`https://github.com/zed-industries/zed.git`); add if missing
- [ ] `git fetch upstream --no-tags && git fetch origin && git checkout main && git pull origin main`
- [ ] Record fence (`git merge-base main upstream/main`), upstream HEAD, commit count, fork-only count, diffstat
- [ ] Confirm ACP pins are still identical on both sides (`=2.0.0` / derive `2.0.0` / schema `1.5.0`)
- [ ] Confirm `rust-toolchain.toml` delta (`1.95.0` → `1.97.1`)
- [ ] Re-enumerate conflicts with `git merge-tree --write-tree --name-only main upstream/main`
- [ ] If the measured numbers differ materially from the spec baseline, note the delta in the porting guide before proceeding

## Phase 2 — Branch and open the porting-guide entry

- [ ] `git checkout -b feature/002701-merge-latest-zed`
- [ ] Insert a `## Merge 002701 (2026-08-10)` section at the top of the merge-history list in `portingguide.md` (above `## Merge 2026-07-29`), with the measured window summary — **before** running the merge
- [ ] Commit the opening porting-guide stub

## Phase 3 — Merge and resolve the textual conflict

- [ ] `git merge upstream/main`
- [ ] Resolve `crates/title_bar/Cargo.toml`: take upstream's `git_ui_core.workspace = true`, keep the Helix `[features] external_websocket_sync = ["dep:external_websocket_sync"]` entry and the `external_websocket_sync = { workspace = true, optional = true }` dependency
- [ ] Append the resolution to the porting guide's `### Conflicts and Resolutions`
- [ ] Resolve any `Cargo.lock` conflict `--theirs`
- [ ] Verify no `<<<<<<<` / `>>>>>>>` markers remain anywhere in the tree
- [ ] Complete the merge commit

## Phase 4 — `git_ui` → `git_ui_core` migration (compile-driven)

- [ ] Confirm the workspace `Cargo.toml` gained `crates/git_ui_core` as a member and `git_ui_core = { path = "crates/git_ui_core" }` as a dep, and still has all Helix members plus `rust-embed` `debug-embed`
- [ ] Migrate `git_ui::worktree_service::*`, `worktree_picker`, `worktree_names`, `askpass_modal`, `created_worktrees`, `file_diff_view`, `notifications` references to `git_ui_core::` in fork-only code — symbol by symbol, not a blanket sed
- [ ] Convert any `git_ui::git_picker::popover(ws, repo, GitPickerTab::Branches, rems(34.), window, cx)` call to `git_ui_core::build_branch_picker(ws, repo, window, cx)` (fewer args, already returns `Option` — drop the `Some(..)` wrapper)
- [ ] Migrate the fork-only file `crates/zed/src/visual_test_runner.rs` by hand (upstream has no fix for it)
- [ ] Add `git_ui_core.workspace = true` to each crate `Cargo.toml` that now needs it; remove `git_ui.workspace = true` only where nothing references the remaining `git_ui` symbols
- [ ] Run the verification grep from `design.md` §"Verification grep" — must be clean
- [ ] Write the moved/stayed symbol map and every migrated call site into the porting guide's `### git_ui → git_ui_core migration` section

## Phase 5 — rustc 1.95 → 1.97.1

- [ ] Confirm `rust-toolchain.toml` took upstream's `channel = "1.97.1"`
- [ ] Verify `/home/retro/work/helix/Dockerfile.zed-build` picks 1.97.1 up from `rust-toolchain.toml` with no edit; if the toolchain or a cross target fails to install, escalate rather than pinning back to 1.95.0
- [ ] Budget for a cold full rebuild (the toolchain change invalidates the Cargo cache) — do not treat the long build as a hang
- [ ] Fix fork-only diagnostics from the newer compiler, following upstream's own remedy where one exists (e.g. `rems_from_px(12.)` → `rems_from_px(12_f32)`)
- [ ] Record the bump, any Dockerfile change, and every fixed diagnostic in the porting guide's `### rustc 1.95 → 1.97` section

## Phase 6 — Audit auto-merged files (priority order)

- [ ] `crates/extensions_ui/src/extensions_ui.rs` (−662, bulk moved to `components/extension_card.rs`): confirm all 3 `// HELIX: External agent` markers survive **and sit in live code**; if the host code moved, follow it to `extension_card.rs` and record the relocation
- [ ] `crates/zed/src/main.rs` (±27): confirm `--headless`, `--allow-multiple-instances`, `initialize_headless()`, `build_application(headless)` still work against upstream's reworked startup (`restore_task.shared()`, `first_window_rx` oneshot, `select_biased!`, `credentials_provider` arg); confirm the headless branch short-circuits before the window-restore machinery
- [ ] `crates/zed/src/zed.rs` (+141): confirm `initialize_agent_panel` and the WebSocket init inside it survive the new action registrations and signature changes
- [ ] `crates/agent_ui/src/agent_panel.rs` (±35): confirm the `git_ui_core` conversions landed and the full-screen `IconButton` rework (`toggle-full-screen` id + `.toggle_state`) did not disturb adjacent Helix cfg blocks
- [ ] `crates/agent_ui/src/conversation_view/thread_view.rs` (±68): confirm Helix's `current_model_id()` three-way fallback survives alongside `restrict_scroll_to_axis()` / `pause_following_tail()`
- [ ] `assets/settings/default.json`: accept upstream's 3 new keys (`git_gutter_width`, `git.diff_base`, `terminal.starts_open`) and the reworded decoration comments; **leave `trust_all_worktrees` / `show_sign_in` at upstream values** (see next phase)
- [ ] `crates/feature_flags/src/flags.rs`: confirm `AcpBetaFeatureFlag::enabled_for_all() -> true` survived upstream's removal of `ProjectPanelUndoRedoFeatureFlag` / `AutoWatchFeatureFlag`
- [ ] Run every confirming grep in `design.md` §"Confirming greps"
- [ ] Walk every numbered item of `portingguide.md` §"Rebase Checklist"

## Phase 7 — Critical Fix and Helix-surface verification

- [ ] Verify Critical Fixes #1–#9 and #11 by grep/read (host files unchanged upstream — expect a clean pass, but confirm)
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` returns 0, including `#[cfg(test)]` code
- [ ] Verify the constraint #8 replacement gate: `cfg!(feature = "external_websocket_sync")` auto-trust in `crates/project/src/trusted_worktrees.rs` and `render_restricted_mode()` returning `None` in `title_bar.rs`
- [ ] Verify the constraint #12 replacement gate: `&& !cfg!(feature = "external_websocket_sync")` on the sign-in button in `title_bar.rs`
- [ ] Verify `ThreadDisplayNotification` still calls `OnboardingUpsell::set_dismissed(true, cx)` and initialises `NativeAgentSessionList`
- [ ] Verify built-in agent hiding stays under `cfg(not(feature = "external_websocket_sync"))`
- [ ] Verify the windowless `cx.subscribe()` in `thread_service.rs` (incremental `message_added` streaming)
- [ ] Verify `from_existing_thread()` matches its `::new` sibling on `ConversationView`
- [ ] Verify `crates/external_websocket_sync/` still has all 10 source entries
- [ ] Record the results in the porting guide's `### Helix-surface survival check`

## Phase 8 — Build and unit tests

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — zero errors
- [ ] Feature-OFF gate: `cargo check -p zed` without `external_websocket_sync` via a one-off Docker run in the build image
- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send`
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized`

## Phase 9 — E2E Docker gate (hard gate — merge is not complete without it)

- [ ] Copy the built binary to `crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] `(cd crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy)`
- [ ] Confirm `ANTHROPIC_API_KEY` is present in the environment; flag immediately if not
- [ ] `./run_docker_e2e.sh` — all **17** phases green for `zed-agent`
- [ ] `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` — all 17 phases green for both agents
- [ ] Confirm UI-state queries (Phase 6) return correct `thread_id`, `entry_count`, `active_view`
- [ ] At most one retry per agent for the known Claude Phase-1 npm-install / API-latency flake; a second failure is a real failure — never use `--no-build` while investigating
- [ ] Record E2E results (including the 17-phase count) in the porting guide

## Phase 10 — Finalise the porting guide

- [ ] `### Conflicts and Resolutions`, `### git_ui → git_ui_core migration`, `### rustc 1.95 → 1.97`, `### Retired / superseded Helix patches`, `### Helix-surface survival check` all complete
- [ ] Supersession note written: constraints #8/#12 moved from `default.json` to `cfg!` gates, so future briefs can drop them
- [ ] Note that `conversation_view.rs`, `agent_servers/src/acp.rs`, `agent/src/agent.rs`, `acp_thread/src/connection.rs` had zero upstream changes this window
- [ ] Commit-history table extended; stale Rebase-Checklist entries corrected; every "16 phases" reference updated to 17

## Phase 11 — Push and bump

- [ ] Re-fetch `upstream/main` and `origin/main`; absorb any out-of-band fork pushes; run an extension merge round if upstream advanced materially during the work
- [ ] `git push -u origin feature/002701-merge-latest-zed` (never force-push `main`)
- [ ] Bump `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` from `77d466a4550ebd1184901f3c6ed5816d3632ab0a` to the new merge HEAD, on a `feature/002701-merge-latest-zed` branch in the helix repo; include any forced `Dockerfile.zed-build` change on the same branch; push
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` into this task directory
- [ ] Do not open PRs from the agent — the Helix UI does that
- [ ] Confirm the helixml/zed CI pipeline is green
