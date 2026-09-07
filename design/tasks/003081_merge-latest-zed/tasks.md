# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork

## Pre-flight
- [ ] Read `003012_merge-latest-zed/` (design.md §4–§7 especially), then `002930` and `002701` for the `git_ui_core` symbol map
- [ ] Read `/home/retro/work/zed/portingguide.md` §"Critical Fixes" and the 2026-07-29 merge section
- [ ] Confirm `ANTHROPIC_API_KEY` is set — the e2e gate cannot run without it; flag immediately if absent
- [ ] Add/confirm `upstream` remote (`https://github.com/zed-industries/zed.git`), fetch `main`
- [ ] Re-measure fence, upstream HEAD, commit count, `git merge-tree --write-tree origin/main upstream/main`; record in the porting guide
- [ ] Cut `feature/003081-merge-latest-zed` from fork `main`

## Merge and conflict resolution
- [ ] `git merge upstream/main` (merge commit — not squash, not rebase)
- [ ] Resolve `crates/agent_servers/src/acp.rs`: take upstream's `register_session` / `observe_release` lifecycle, delete the fork's `ref_count` arithmetic, keep `session_creation_chain` / `SessionCreationGuard` (PR #50)
- [ ] Re-express `force_close_session` (PR #63) against the new model: drop the `pending_sessions`/`sessions` entry, then send `CloseSessionRequest`
- [ ] Union-resolve the `acp.rs` test hunk — keep `test_concurrent_session_creation_is_serialized` **and** upstream's `release_dropped_entities`
- [ ] Verify `crates/acp_thread/src/connection.rs` keeps the fork's `force_close_session` trait method (upstream left the file unchanged)
- [ ] Resolve `crates/http_client_tls/Cargo.toml` as a union (`rustls-pki-types` + `webpki-roots` + `log`)
- [ ] Resolve `crates/title_bar/Cargo.toml`: keep the optional `external_websocket_sync` dep/feature, drop `git_ui`, take `git_ui_core`
- [ ] Resolve `crates/agent_ui/Cargo.toml`: keep `time_format` + `tokio`, take upstream's removal of `time`
- [ ] Resolve `crates/zed/Cargo.toml`: keep `tokio`/`ztracing`/`tracing` + Helix features, take version `1.20.0` and upstream's dep reshuffle
- [ ] Resolve `crates/zed/src/main.rs` to `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`
- [ ] Resolve `Cargo.lock` `--theirs`, regenerate via the build
- [ ] Confirm zero `<<<<<<<` / `>>>>>>>` markers in the tree
- [ ] Write the porting guide's `### ACP session lifecycle` and `### Conflicts and Resolutions` sections **now**, before moving on

## git_ui → git_ui_core migration
- [ ] Re-point moved symbols (`worktree_service`, `created_worktrees`, `worktree_picker`, `worktree_names`, `notifications`, `askpass_modal`, `file_diff_view`) at `git_ui_core` across the 13 fork files, compile-driven
- [ ] Leave stayed symbols (`init`, `git_panel`, `git_picker`, `git_graph`, `project_diff`, `solo_diff_view`, `staged_diff`, `unstaged_diff`) on `git_ui`
- [ ] Migrate `git_picker::popover(...)` → `git_ui_core::build_branch_picker(workspace, repo, window, cx)`, dropping the `Some(..)` wrapper
- [ ] Add `git_ui_core.workspace = true` where needed; remove `git_ui` only where nothing references it
- [ ] Write the porting guide's `### git_ui → git_ui_core migration` section with the full symbol map

## Toolchain and crate churn
- [ ] Take upstream `rust-toolchain.toml` `channel = "1.97.1"`
- [ ] Verify `/home/retro/work/helix/Dockerfile.zed-build` resolves 1.97.1 with no manual pin edit; flag immediately if the toolchain fetch fails
- [ ] Fix fork-only compiler diagnostics from the bump, following upstream's own remedy where one exists
- [ ] Confirm `git_ui_core`, `gpui_apple`, `tabular_data_preview`, `call_hierarchy`, `language_detection` are workspace members and build
- [ ] Confirm `csv_preview`, `rich_text`, `supermaven`, `supermaven_api`, `panel` are gone; drop `panel.workspace` from `crates/git_ui/Cargo.toml` and `csv_preview.workspace` from `crates/zed/Cargo.toml`
- [ ] Grep the workspace for residual `panel::` / `rich_text::` / `supermaven` references
- [ ] Confirm workspace `members` still lists every Helix crate (`external_websocket_sync`, `sidebar`, …)
- [ ] Write the porting guide's `### rustc 1.95 → 1.97` and `### Crate churn` sections

## Auto-merge audit
- [ ] P1 `crates/agent/src/agent.rs` (+732) — Fix #1 `pending_sessions` shared task, `wait_for_tools_ready`
- [ ] P2 `crates/zed/src/zed.rs` (+1096) — `initialize_agent_panel` and the WebSocket init inside it
- [ ] P3 `crates/agent_ui/src/agent_panel.rs` (+585) — `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query callback, `from_existing_thread`, `ThreadDisplayNotification`, Fix #11
- [ ] P4 `crates/agent_ui/src/conversation_view.rs` (±134) — Fix #2; confirm the `supports_close_session()` guard still behaves against trait defaults
- [ ] P5 `crates/title_bar/src/title_bar.rs` (+110) — `render_restricted_mode()` → `None` and sign-in suppression under the feature gate
- [ ] P6 `crates/extensions_ui/src/extensions_ui.rs` (−662) — re-apply the 3× `// HELIX: External agent` markers in `components/extension_card.rs` if upstream moved the host; verify they are in a live path
- [ ] P7 `crates/reqwest_client/src/reqwest_client.rs` (±73) — `ZED_HTTP_INSECURE_TLS`
- [ ] P8 `crates/agent_ui/src/conversation_view/thread_view.rs` (±88) — `current_model_id()` fallback
- [ ] P9 `crates/anthropic/src/anthropic.rs` (+561) — take upstream ordering wholesale
- [ ] P10–P14: `http_client_tls.rs`, `grep_tool.rs` (`MAX_LINE_CHARS = 500`), `flags.rs` (`AcpBetaFeatureFlag::enabled_for_all() -> true`), `zed/src/main.rs` headless flags, `open_ai.rs`
- [ ] Confirming greps: `acp_thread/src/connection.rs`, `agent_ui/src/acp/**`, `project/src/trusted_worktrees.rs`, `external_websocket_sync/**`
- [ ] Verify all 11 Critical Fixes plus PR #63 reachability from `thread_service.rs`
- [ ] Verify ACP pin unchanged (`=2.0.0`, schema `1.5.0`) and `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` returns 0
- [ ] Verify elicitation types in `acp_thread.rs` unchanged; leave `feature/002731-agent-questions` untouched
- [ ] Verify constraints #8/#12 remain cfg gates — do **not** restore JSON keys
- [ ] Write the porting guide's `### Helix-surface survival check` section

## Build and test gates
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — zero errors
- [ ] Feature-off `cargo check -p zed` via a one-off Docker run in the build image
- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send`
- [ ] `cargo test -p agent_servers` — serialization test plus upstream's reworked session-lifecycle tests
- [ ] `go mod tidy` in `crates/external_websocket_sync/e2e-test/helix-ws-test-server/`
- [ ] `run_docker_e2e.sh` — all 17 phases green for `zed-agent`
- [ ] `E2E_AGENTS="zed-agent,claude"` — all 17 phases green for `claude` (one retry allowed for the known Phase-1 npm flake)

## Wrap-up
- [ ] Finish the porting guide section: window summary, superseded patches, commit-history table, and the note that 002701 / 002930 / 003012 were planned but never executed
- [ ] Re-fetch `upstream/main` and `origin/main`; run an extension round if upstream moved materially
- [ ] Push `feature/003081-merge-latest-zed` to `origin`; do not force-push `main` or touch `origin/helix-fork`
- [ ] Bump `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` to the merge HEAD on a `feature/003081-merge-latest-zed` branch and push
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` into this task directory
- [ ] Confirm helixml/zed CI pipeline green (`build-zed`, `zed-e2e-image`, `zed-e2e-headless-smoke`, `zed-e2e-protocol-lifecycle`, `zed-e2e-live-claude-latest`)
