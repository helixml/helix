# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork

## Phase 0 — Pre-work

- [ ] Read `helix-specs/design/tasks/002930_merge-latest-zed/` (all three files)
- [ ] Read `helix-specs/design/tasks/002701_merge-latest-zed/design.md` — the `git_ui` → `git_ui_core` symbol map
- [ ] Read `/home/retro/work/zed/portingguide.md` §"Critical Fixes" and the `2026-07-29` merge section
- [ ] Confirm `ANTHROPIC_API_KEY` is present in the environment; flag immediately if not (E2E gate blocks on it)
- [ ] Confirm `docker` works and `./stack build-zed dev` is runnable

## Phase 1 — Re-measure

- [ ] Ensure `upstream` remote = `https://github.com/zed-industries/zed.git`; `git fetch upstream main`
- [ ] `git fetch origin` and confirm fork `main` has not moved past `1e0be14e6c`
- [ ] Re-record fence (`git merge-base origin/main upstream/main`), upstream HEAD, commit count, fork-only count
- [ ] Re-run `git merge-tree --write-tree --name-only origin/main upstream/main` and confirm the six-file conflict list
- [ ] Re-run `git diff --stat <fence> upstream/main` on the hot files and re-derive audit priorities if they moved
- [ ] Confirm ACP is still `=2.0.0` / derive `2.0.0` / schema `1.5.0` on both sides

## Phase 2 — Merge and resolve the six conflicts

- [ ] Cut `feature/003012-merge-latest-zed` from `origin/main`
- [ ] `git merge upstream/main` (merge — not squash, not rebase)
- [ ] Resolve `crates/http_client_tls/Cargo.toml` — union: keep fork `rustls-pki-types = "1"` + upstream `log` and `webpki-roots`
- [ ] Resolve `crates/title_bar/Cargo.toml` — upstream shape (`git_ui_core`, drop `git_ui`/`notifications`/windows block, dev-dep reshuffle) + re-insert both fork `external_websocket_sync` lines
- [ ] Resolve `crates/agent_ui/Cargo.toml` — upstream shape (drop `git_ui`/`time`, add `git_ui_core`, prune dev-deps) + keep `time_format`, `tokio`, `external_websocket_sync_dep`, the `[features]` entry
- [ ] Resolve `crates/zed/Cargo.toml` — upstream version `1.19.0`, `inspector` feature, dependency reshuffle + keep `tokio`, `ztracing`, `external_websocket_sync` dep and `[features]` entries
- [ ] Resolve `crates/zed/src/main.rs` — `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`; both error-path `build_application(false)` sites kept
- [ ] Resolve `Cargo.lock` `--theirs`, regenerate via the build
- [ ] Verify zero `<<<<<<<` / `>>>>>>>` markers anywhere in the tree
- [ ] **Write the `### Conflicts and Resolutions` porting-guide subsection now**

## Phase 3 — `git_ui` → `git_ui_core` migration

- [ ] Enumerate all `git_ui::` references (13 files at baseline) and classify each symbol as moved vs stayed
- [ ] Re-point moved-symbol references at `git_ui_core::`; leave `git_panel`/`branch_diff`/`commit_view`/`project_diff` on `git_ui`
- [ ] Migrate `git_ui::git_picker::popover(...)` call sites to `git_ui_core::build_branch_picker(workspace, repo, window, cx)`, dropping the `Some(..)` wrapper
- [ ] Add `git_ui_core.workspace = true` to every fork crate that now needs it; remove `git_ui` only where nothing references it
- [ ] **Write the `### git_ui → git_ui_core migration` porting-guide subsection with the full symbol map** (highest-value artefact this window)

## Phase 4 — rustc 1.95.0 → 1.97.1

- [ ] Take upstream's `rust-toolchain.toml` (`channel = "1.97.1"`)
- [ ] Verify `/home/retro/work/helix/Dockerfile.zed-build` resolves and installs 1.97.1 with no manual pin edit
- [ ] Budget for a cold full rebuild (the toolchain change invalidates the Cargo cache)
- [ ] Fix new-compiler diagnostics in fork-only code, following upstream's remedy where one exists
- [ ] **Write the `### rustc 1.95 → 1.97` porting-guide subsection**

## Phase 5 — Crate churn

- [ ] Confirm `git_ui_core`, `gpui_apple`, `tabular_data_preview`, `call_hierarchy`, `language_detection` are workspace members
- [ ] Confirm `csv_preview`, `rich_text`, `supermaven`, `supermaven_api` are removed and nothing in the fork references them
- [ ] Review the auto-merged workspace `Cargo.toml` — all Helix members (`external_websocket_sync`, `sidebar`, …) preserved
- [ ] **Write the `### Crate churn` porting-guide subsection**

## Phase 6 — Auto-merge audit (priorities re-derived from fresh `--stat`)

- [ ] P1 `crates/zed/src/zed.rs` (+816) — `initialize_agent_panel` and WebSocket init survive
- [ ] P2 `crates/agent_ui/src/agent_panel.rs` (+549) — `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query callback, `acp_history_store()`, `from_existing_thread`, `ThreadDisplayNotification` handler, Fix #11 guard
- [ ] P3 `crates/agent/src/agent.rs` (+363) — Fix #1: `pending_sessions` shared-task in `load_session()`, `wait_for_tools_ready`
- [ ] P4 `crates/agent_ui/src/conversation_view.rs` (±125) — Fix #2: no duplicate WebSocket sends
- [ ] P5 `crates/title_bar/src/title_bar.rs` (+108) — `render_restricted_mode()` returns `None` under the gate; sign-in suppression via `&& !cfg!(feature = "external_websocket_sync")`
- [ ] P6 `crates/extensions_ui/src/extensions_ui.rs` (−662) — 3× `// HELIX: External agent` markers present **and in a live code path**; re-apply in `components/extension_card.rs` if upstream moved their host
- [ ] P7 `crates/reqwest_client/src/reqwest_client.rs` (±73) — `ZED_HTTP_INSECURE_TLS` intact
- [ ] P8 `crates/http_client_tls/src/http_client_tls.rs` (±11) — fork `rustls-pki-types` usage coexists with upstream `webpki-roots`
- [ ] P9 `crates/agent_ui/src/conversation_view/thread_view.rs` (±88) — Helix `current_model_id()` fallback intact
- [ ] P10 `crates/anthropic/src/anthropic.rs` (+502) — take upstream model ordering wholesale
- [ ] P11 `crates/agent_servers/src/acp.rs` (±33) — `SessionCreationGuard` / `session_creation_chain` (PR #50) intact
- [ ] P12 `crates/agent/src/tools/grep_tool.rs` (±12) — `truncate_long_lines()` / `MAX_LINE_CHARS = 500` intact
- [ ] P13 `crates/zed/src/main.rs` — `--headless`, `--allow-multiple-instances`, `initialize_headless()`, `build_application(headless)` intact around the new `restart_arguments` flow
- [ ] P14 `crates/language_models/src/provider/open_ai.rs` (±18) — confirming read
- [ ] Confirming grep only (verified byte-unchanged upstream): `acp_thread/src/connection.rs`, `agent_ui/src/acp/**`, `project/src/trusted_worktrees.rs`, `external_websocket_sync/**`
- [ ] Confirm remaining Critical Fixes #3, #4, #5, #6, #7, #8, #9 by grep
- [ ] **Write the `### Helix-surface survival check` porting-guide subsection**

## Phase 7 — Helix-specific surface and invariants

- [ ] `crates/external_websocket_sync/` intact — all 10 source files present
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override still attached after upstream's `flags.rs` (−27) pruning
- [ ] `ThreadDisplayNotification` handler calls `OnboardingUpsell::set_dismissed(true, cx)` and initialises `NativeAgentSessionList`
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) still under `cfg(not(feature = "external_websocket_sync"))`
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved (incremental `message_added` streaming)
- [ ] Verify the cfg gates for constraints #8/#12 — **not** `assets/settings/default.json`; do not restore `trust_all_worktrees` / `show_sign_in` JSON
- [ ] `title_bar` `external_websocket_sync` dep stays `optional = true`; workspace `rust-embed` keeps `debug-embed`; `wait_for_tools_ready` uses `cx.background_executor().timer()`
- [ ] `dev_container_suggest.rs` early return; migration banner `Hidden`; trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches exhaustive; Fix 1b draft-suppression `return;` is still the FIRST statement of its `BaseView::Uninitialized` branch
- [ ] Elicitation types in `crates/acp_thread/src/acp_thread.rs` unchanged after the ±16 auto-merge
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` returns 0
- [ ] **Write the `### Retired / superseded Helix patches` porting-guide subsection**

## Phase 8 — Build and test gates

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — zero errors
- [ ] Feature-off compile: one-off Docker `cargo check -p zed` **without** `external_websocket_sync`
- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] `go mod tidy` in `crates/external_websocket_sync/e2e-test/helix-ws-test-server/`
- [ ] **E2E hard gate**: `run_docker_e2e.sh` — all 17 phases green for `zed-agent`
- [ ] **E2E hard gate**: all 17 phases green with `E2E_AGENTS="zed-agent,claude"` (one retry per agent allowed for the known Claude Phase-1 flake)

## Phase 9 — Finish the porting guide

- [ ] Confirm `## Merge 003012 (2026-08-31)` sits **above** `## Merge 2026-07-29` (~line 743)
- [ ] Window summary complete: fence SHA, upstream HEAD SHA, commit count, window length, ACP unchanged, rustc bump, zed `1.15.0 → 1.19.0`
- [ ] Extend the commit-history table; correct stale Rebase-Checklist entries
- [ ] Record that **002701 and 002930 were planned but never executed** — why this window is 434 commits

## Phase 10 — Ship

- [ ] Re-fetch `upstream/main` and `origin/main`; absorb any out-of-band fork pushes; run an extension round if upstream advanced materially
- [ ] Push `feature/003012-merge-latest-zed` to `origin` (mirrors `helixml/zed`)
- [ ] Bump `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` from `6f9300a70db9126b5f03deeb883c19adc21d545b` to the new merge HEAD, on a `feature/003012-merge-latest-zed` branch in the helix repo; push
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` into this task directory
- [ ] Confirm helixml/zed CI green (drone `build-zed` + `zed-e2e-test`)
- [ ] Confirm `main` was not force-pushed, `origin/helix-fork` untouched, and no PRs were opened by the agent
