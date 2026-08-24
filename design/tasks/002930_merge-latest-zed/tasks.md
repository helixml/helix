# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork

## 0. Pre-work

- [ ] Read `helix-specs/design/tasks/002701_merge-latest-zed/` (all three files) — it was never executed and is still the primary reference, especially design.md §"Work item 1: `git_ui` → `git_ui_core`"
- [ ] Read `helix-specs/design/tasks/002353_merge-latest-zed/` for anything 002701 did not carry forward
- [ ] Read `/home/retro/work/zed/portingguide.md` §"Critical Fixes (Must Be Preserved)" and §"Rebase Checklist"
- [ ] Confirm `ANTHROPIC_API_KEY` is present in the environment — flag immediately if not, the merge cannot be signed off without the E2E gate

## 1. Set up and re-measure

- [ ] `cd /home/retro/work/zed && git fetch origin` — confirm `origin/main` is still `7357327f2d` (absorb any newer fork pushes)
- [ ] `git remote add upstream https://github.com/zed-industries/zed.git && git fetch upstream --no-tags` (read-only; not configured by default)
- [ ] Re-measure: fence (`git merge-base origin/main upstream/main`), upstream HEAD, `git log --oneline origin/main..upstream/main | wc -l`
- [ ] Re-run `git merge-tree --write-tree --name-only origin/main upstream/main` and diff the conflict list against the five files this spec recorded
- [ ] Re-confirm ACP versions unchanged (`agent-client-protocol =2.0.0`, derive 2.0.0, schema 1.5.0 on both sides)
- [ ] `git checkout -b feature/002930-merge-latest-zed origin/main`

## 2. Start the porting-guide entry (before any conflict is touched)

- [ ] Insert `## Merge 002930 (2026-08-24)` into `portingguide.md` above `## Merge 2026-07-29 (upstream catch-up, 764 commits)` (line ~744)
- [ ] Write the measured window summary: fence SHA, upstream HEAD SHA, commit count, 26-day window, ACP unchanged, rustc `1.95.0 → 1.97.1`, zed `1.15.0 → 1.18.0`
- [ ] Add a note that 002701 was planned on 2026-08-10 but never executed, explaining the 331-commit window
- [ ] Commit this stub before merging, so the guide is genuinely updated continuously

## 3. Merge and resolve the five textual conflicts

- [ ] `git merge upstream/main` — merge, **not** squash, **not** rebase
- [ ] `Cargo.lock` — resolve `--theirs`; regenerate via the build, never hand-edit
- [ ] `crates/title_bar/Cargo.toml` — keep `external_websocket_sync = { workspace = true, optional = true }`, take `git_ui_core.workspace = true`, drop `git_ui.workspace = true`; verify the `[features]` `external_websocket_sync = ["dep:external_websocket_sync"]` entry survives
- [ ] `crates/agent_ui/Cargo.toml` — take upstream's shape (drops `git_ui`, drops `time`, reorders `fuzzy`/`git`, adds `git_ui_core`, drops dev-dep `clock`); re-insert fork-only `time_format` and `tokio`; re-add `time` only if the compiler demands it
- [ ] `crates/zed/Cargo.toml` — take upstream's `version = "1.18.0"`, the new `inspector` feature block and the reshuffled dependency list; re-insert fork-only `tokio` and `ztracing`; keep every Helix `[features]` entry
- [ ] `crates/zed/src/main.rs` — resolve to `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`
- [ ] `grep -rn '<<<<<<<\|>>>>>>>' crates/ assets/ Cargo.toml` returns nothing
- [ ] Record every resolution in the `### Conflicts and Resolutions` subsection as you go

## 4. `git_ui` → `git_ui_core` migration (compile-driven)

- [ ] Confirm workspace `Cargo.toml` gained `crates/git_ui_core`, `crates/gpui_apple`, `crates/tabular_data_preview` as members and dropped `crates/csv_preview`, while keeping all Helix members
- [ ] Walk the 13 fork files referencing `git_ui::` and re-point only the **moved** symbols at `git_ui_core::` (moved: `worktree_service`, `worktree_picker`, `worktree_names`, `askpass_modal`, `created_worktrees`, `file_diff_view`, `notifications`)
- [ ] Leave symbols that stayed in `git_ui` alone (`git_panel`, `branch_diff`, `commit_view`, `project_diff`)
- [ ] Migrate `git_ui::git_picker::popover(workspace, repo, GitPickerTab::Branches, rems(34.), window, cx)` call sites to `git_ui_core::build_branch_picker(workspace, repo, window, cx)` — different arity, already returns `Option`, drop the `Some(..)` wrapper
- [ ] Add `git_ui_core.workspace = true` to every fork crate that now needs it; remove `git_ui` from a `Cargo.toml` only when nothing there references it
- [ ] Verification grep for moved symbols still under `git_ui::` returns 0
- [ ] Write the full symbol map and every migrated call site into the `### git_ui → git_ui_core migration` subsection

## 5. rustc 1.95.0 → 1.97.1

- [ ] Confirm `rust-toolchain.toml` is on upstream's `channel = "1.97.1"`
- [ ] Verify `/home/retro/work/helix/Dockerfile.zed-build` resolves and installs 1.97.1 from `rust-toolchain.toml` with no manual pin edit; flag immediately if the rustup cache mount or base image cannot
- [ ] Budget and run a cold full rebuild — the toolchain change invalidates the entire Cargo cache
- [ ] Fix new-compiler diagnostics in fork-only code, following upstream's own remedy where one exists (e.g. `rems_from_px(12.)` → `rems_from_px(12_f32)`)
- [ ] Record the bump and every fork-only diagnostic in the `### rustc 1.95 → 1.97` subsection

## 6. Audit the auto-merged files (auto-merged ≠ correct)

- [ ] **P1** `extensions_ui/src/extensions_ui.rs` (−662, bulk moved to `components/extension_card.rs`) — the 3× `// HELIX: External agent` markers exist and are in a live code path; re-apply in the new file if upstream moved their host, and record the move
- [ ] **P2** `agent/src/agent.rs` (+43 in `NativeAgentConnection`) — Fix #1 `pending_sessions` shared-task in `load_session()`, `wait_for_tools_ready` using `cx.background_executor().timer()` (no `smol::Timer`)
- [ ] **P3** `zed/src/zed.rs` (+382, ~90% in `mod tests`) — `initialize_agent_panel` and the WebSocket init inside it intact
- [ ] **P4** `agent_ui/src/agent_panel.rs` (+118, only 2 non-test hunks) — `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query callback, `acp_history_store()`, Fix #11 entity-identity guard in `load_agent_thread`
- [ ] **P4b** `ThreadDisplayNotification` handler still calls `OnboardingUpsell::set_dismissed(true, cx)` and initialises `NativeAgentSessionList`
- [ ] **P5** `agent_ui/src/conversation_view/thread_view.rs` (±68) — Helix `current_model_id()` 3-way fallback survives `restrict_scroll_to_axis()` / `pause_following_tail()`
- [ ] **P6** `anthropic/src/anthropic.rs` (+330) — upstream model list and ordering taken wholesale, not hand-merged
- [ ] **P7** `agent_servers/src/acp.rs` (±33) — `SessionCreationGuard` and `session_creation_chain` (PR #50) intact
- [ ] **P8** `zed/src/main.rs` — `--headless`, `--allow-multiple-instances`, `initialize_headless()`, `build_application(headless)` intact around upstream's new restart/restore flow
- [ ] **P9** `feature_flags/src/flags.rs` (−27) — `AcpBetaFeatureFlag::enabled_for_all() -> true` survived upstream's flag pruning
- [ ] **P10** `title_bar/src/title_bar.rs` (±16) — `render_restricted_mode()` → `None` gate and the sign-in cfg gate intact
- [ ] **P11** `assets/settings/default.json` (±52) — do **not** re-add `trust_all_worktrees` / `show_sign_in`; audit the cfg gates in `project/src/trusted_worktrees.rs` and `title_bar.rs` instead
- [ ] **P12/P13** `acp_thread/src/acp_thread.rs` (±7) and workspace `Cargo.toml` — Critical Fixes untouched; `rust-embed` keeps `debug-embed`
- [ ] Confirming grep only (unchanged upstream): `acp_thread/src/connection.rs`, `crates/external_websocket_sync/**`, `agent_ui/src/conversation_view.rs`, `reqwest_client/src/reqwest_client.rs`, `agent/src/tools/grep_tool.rs`, `language_models/src/provider/open_ai.rs`
- [ ] Walk the remaining Critical Fixes #2–#10 as a grep pass and tick each off in the guide

## 7. Helix-surface invariants

- [ ] `crates/external_websocket_sync/` intact — all 10 source files present, crate unchanged by the merge
- [ ] All custom code remains wrapped in `cfg(feature = "external_websocket_sync")`
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) still under `cfg(not(feature = "external_websocket_sync"))`
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved (`subscribe_in` is silently dropped without a window context)
- [ ] Fix 1b cfg-gated draft-suppression `return;` is still the FIRST statement of its `BaseView::Uninitialized` branch
- [ ] `grep_tool.rs` `truncate_long_lines()` / `MAX_LINE_CHARS = 500` intact
- [ ] `reqwest_client.rs` / `http_client_tls.rs` `ZED_HTTP_INSECURE_TLS` support intact
- [ ] `dev_container_suggest.rs` early return, migration banner `Hidden`, trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches still exhaustive
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` returns 0
- [ ] Record the `### Helix-surface survival check` and `### Retired / superseded Helix patches` subsections

## 8. Build and test

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — zero errors (expect a cold rebuild)
- [ ] Feature-OFF build passes via a one-off docker run in the build image
- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] `go mod tidy` in `crates/external_websocket_sync/e2e-test/helix-ws-test-server/`
- [ ] **E2E hard gate:** `./run_docker_e2e.sh` — all 17 phases green for `zed-agent` (never use `--no-build` when investigating)
- [ ] **E2E hard gate:** `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` — all 17 phases green for `claude`; one retry permitted for the known Phase-1 npm/API flake, a second failure is real
- [ ] Add `### Crate churn` (git_ui_core / gpui_apple / tabular_data_preview added, csv_preview removed) to the guide

## 9. Ship

- [ ] Re-fetch `upstream/main` and `origin/main`; absorb any out-of-band fork pushes; run an extension merge round if upstream advanced materially during the work
- [ ] Extend the porting guide's commit-history table and correct stale Rebase-Checklist entries
- [ ] Push `feature/002930-merge-latest-zed` to `origin` (mirrors helixml/zed); do **not** force-push `main`; leave the dead `origin/helix-fork` untouched
- [ ] Bump `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` from `71a2940881e37fff3ca099cb49ae15ce4b996f9a` to the new merge HEAD, on branch `feature/002930-merge-latest-zed`, and push
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` into this task directory; do not open PRs (the Helix UI does that)
- [ ] Confirm the helixml/zed CI pipeline is green (drone `build-zed` and `zed-e2e-test` steps)

## 10. Follow-up (not this task, but flag it)

- [ ] Notify the owner of `origin/feature/002731-agent-questions` (elicitations + e2e Phase 18) to rebase onto the merged `main`; `crates/external_websocket_sync/**` is unchanged upstream this window, so the rebase should be near-trivial
