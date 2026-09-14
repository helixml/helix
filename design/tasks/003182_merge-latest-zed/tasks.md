# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork

## Phase 0 — Read and re-measure (do not skip; the fork moved since 003081)

- [ ] Read `helix-specs/design/tasks/003081_merge-latest-zed/` in full (requirements + design)
- [ ] Skim `003012`, `002930`, `002701` for anything 003081 did not carry forward
- [ ] Read `/home/retro/work/zed/portingguide.md` — §"Critical Fixes", §"Rebase Checklist", §"Merge 2026-07-29"
- [ ] `cd /home/retro/work/zed && git remote add upstream https://github.com/zed-industries/zed.git` (if absent) and `git fetch upstream main`
- [ ] Re-measure: fence, upstream HEAD, `git rev-list --count main..upstream/main`, ACP pin, `rust-toolchain.toml`, zed crate version
- [ ] Re-run `git merge-tree --write-tree main upstream/main` and confirm the conflict set is still the 8 files in requirements.md
- [ ] Re-read the **fork side** of `crates/agent_servers/src/acp.rs` — PRs #93/#95/#96 landed after 003081 was written
- [ ] Confirm `ANTHROPIC_API_KEY` is available (env or `../helix/.env`); flag immediately if not
- [ ] Cut `feature/003182-merge-latest-zed` from fork `main`

## Phase 1 — ACP 2.1.0 risk check (do this BEFORE merging)

- [ ] Take upstream's `agent-client-protocol = { version = "=2.1.0", features = ["unstable"] }` pin on a scratch branch
- [ ] Verify `JsonRpcNotification` / `JsonRpcRequest` / `JsonRpcResponse` derive macros still exist in 2.1.0
- [ ] If they are gone: STOP, surface to the user as a scope change, and record in the porting guide
- [ ] Verify the `ErrorCode` variants the fork uses still exist in 2.1.0

## Phase 2 — Merge in rounds

- [ ] Split the 616 commits into ~4 rounds at upstream merge commits
- [ ] Round 1: `git merge <round-1-tip>` — resolve, `cargo check`, commit, update porting guide
- [ ] Round 2: same
- [ ] Round 3: same
- [ ] Round 4: merge to `upstream/main` HEAD — resolve, `cargo check`, commit, update porting guide
- [ ] Confirm zero `<<<<<<<` / `>>>>>>>` markers in the tree
- [ ] Confirm all 357 fork-only commits are still reachable

## Phase 3 — Conflict resolutions

- [ ] `crates/agent_servers/src/acp.rs`: take upstream's `register_session` / `observe_release` / `agent_supports_session_close()`; delete fork `ref_count` bookkeeping
- [ ] `acp.rs`: keep `RawSessionNotification` + `handle_raw_session_notification` registration
- [ ] `acp.rs`: keep `RawRequestPermissionRequest` / `RawRequestPermissionResponse` with top-level `answers`
- [ ] `acp.rs`: keep Codex `jetbrains` / `nativeSubagentSessions` meta in `client_capabilities_for_agent`
- [ ] `acp.rs`: keep `AcpBetaFeatureFlag` / `FeatureFlagAppExt` import and use
- [ ] `acp.rs`: keep `session_creation_chain` / `SessionCreationGuard` / `acquire_session_creation_slot` (PR #50)
- [ ] `acp.rs`: re-express `force_close_session` (PR #63) — remove the map entry, then send `CloseSessionRequest`
- [ ] `acp.rs`: keep `[ACP_SPAWN]` diagnostic logging
- [ ] `acp.rs`: union-resolve tests — both `test_concurrent_session_creation_is_serialized` and upstream's `release_dropped_entities` survive
- [ ] `crates/recent_projects/src/dev_container_suggest.rs`: fork's `suggest_dev_container` guard stays FIRST; upstream body taken wholesale
- [ ] `crates/http_client_tls/Cargo.toml`: union `rustls-pki-types` + `webpki-roots` / `log`
- [ ] `crates/title_bar/Cargo.toml`: keep `external_websocket_sync` optional dep; drop `git_ui`, take `git_ui_core`
- [ ] `crates/agent_ui/Cargo.toml`: keep `time_format` + `tokio` + feature/dep; take upstream's `time` removal
- [ ] `crates/zed/Cargo.toml`: keep `tokio`/`ztracing`/`tracing`/`external_websocket_sync`; take version `1.21.0`
- [ ] `crates/zed/src/main.rs`: union `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`
- [ ] `Cargo.lock`: resolve `--theirs`, regenerate via the build
- [ ] `crates/acp_thread/src/connection.rs`: confirm fork's `force_close_session` trait method retained

## Phase 4 — ACP 2.1.0 struct-literal sweep

- [ ] Sweep fork-only ACP construction sites in `crates/external_websocket_sync/src/thread_service.rs` for struct literals; convert any to builders
- [ ] Confirm `CreateElicitationRequest`, `ElicitationFormMode`, `ElicitationSchema`, `ElicitationContentValue`, `ElicitationAcceptAction`, `CreateElicitationResponse`, `ElicitationAction`, `PlanEntryStatus`, `Meta`, `SessionId`, `ToolCallId`, `RequestId`, `PermissionOptionKind` all compile
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` returns 0

## Phase 5 — `git_ui` → `git_ui_core` migration

- [ ] Re-point all 13 fork files referencing `git_ui::`, symbol by symbol, compile-driven (never a blanket rename)
- [ ] Migrate `git_ui::git_picker::popover(...)` → `git_ui_core::build_branch_picker(workspace, repo, window, cx)` (different arity and return type; drop the `Some(..)`)
- [ ] Leave `git_ui::init(cx)` as-is
- [ ] Add `git_ui_core.workspace = true` to each crate that needs it; remove `git_ui` only where nothing uses it

## Phase 6 — Toolchain and crate churn

- [ ] `rust-toolchain.toml` takes upstream's `channel = "1.98.1"`
- [ ] Verify `/home/retro/work/helix/Dockerfile.zed-build` resolves 1.98.1 with no manual pin edit
- [ ] Fix new-compiler diagnostics in fork-only code, following upstream's remedy where one exists
- [ ] Confirm added crates build: `git_ui_core`, `gpui_apple`, `tabular_data_preview`, `call_hierarchy`, `language_detection`, `lsp_command_selector`
- [ ] Confirm removed crates are gone: `csv_preview`, `rich_text`, `supermaven`, `supermaven_api`, `panel`
- [ ] Drop `panel.workspace = true` from `crates/git_ui/Cargo.toml`; grep for residual `panel::` use
- [ ] Drop `csv_preview.workspace = true` from `crates/zed/Cargo.toml`
- [ ] Confirm `[workspace] members` keeps `external_websocket_sync`, `sidebar` and every other Helix member

## Phase 7 — Auto-merge audit (auto-merged ≠ correct)

- [ ] P1 `crates/zed/src/zed.rs` (+972/−136) — `initialize_agent_panel` + WebSocket init survive
- [ ] P2 `crates/agent_ui/src/agent_panel.rs` (+679/−43) — `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query callback, `acp_history_store()`, `from_existing_thread`, `ThreadDisplayNotification`, Fix #11
- [ ] P3 `crates/agent/src/agent.rs` (+585/−151) — Fix #1 `pending_sessions` shared-task, `wait_for_tools_ready`
- [ ] P4 `crates/acp_thread/src/acp_thread.rs` (+1263/−17) — elicitation types unchanged; upstream's `update_idle_sleep_prevention` wiring coexists with the fork's subscribers; `AcpThreadEvent` variant set is a superset of what `thread_service.rs` matches
- [ ] P5 `crates/anthropic/src/anthropic.rs` (+495/−73) — take upstream ordering wholesale
- [ ] P6 `crates/extensions_ui/src/extensions_ui.rs` (+68/−608) — 3× `// HELIX: External agent` markers present and in a live path (check `components/extension_card.rs`)
- [ ] P7 `crates/agent_ui/src/conversation_view.rs` (+149/−42) — Fix #2; compiles against upstream's trait-default `close_session`
- [ ] P8 `crates/agent_ui/src/conversation_view/thread_view.rs` (+97/−63) — Helix `current_model_id()` fallback
- [ ] P9 `crates/title_bar/src/title_bar.rs` (+85/−25) — `render_restricted_mode()` → `None` under the feature gate; sign-in suppression survives
- [ ] P10 `crates/reqwest_client/src/reqwest_client.rs` (+66/−7) — `ZED_HTTP_INSECURE_TLS`
- [ ] P11 `assets/settings/default.json` (+150/−23) — no Helix key dropped; upstream's new keys parse
- [ ] P12 `crates/feature_flags/src/flags.rs` (+0/−27) — `AcpBetaFeatureFlag::enabled_for_all() -> true`
- [ ] P13 `crates/zed/src/main.rs` — `--headless`, `--allow-multiple-instances`, `initialize_headless()`
- [ ] P14 `crates/agent/src/tools/grep_tool.rs` (+7/−5) — `truncate_long_lines()` / `MAX_LINE_CHARS = 500`
- [ ] P15 `crates/language_models/src/provider/open_ai.rs` (+18/−5) — light read
- [ ] P16 `crates/http_client_tls/src/http_client_tls.rs` (+10/−1) — `rustls-pki-types` compiles alongside `webpki-roots`
- [ ] Confirming greps: `acp_thread/src/connection.rs`, `project/src/trusted_worktrees.rs`, `external_websocket_sync/**`

## Phase 8 — Helix surface and Critical Fixes

- [ ] Critical Fixes #1–#9 and #11 all verified present (#10 stays RETIRED)
- [ ] `force_close_session` still reachable from `thread_service.rs:3691`
- [ ] `crates/external_websocket_sync/` intact — 10 source files, 77 test attributes
- [ ] Elicitation relay intact: `ElicitationRequested` → `register_elicitation_question`, `ElicitationResponded` → `resolve_elicitation_question`, plus `elicitation_questions()` / `elicitation_response_content()` / `qwen_questions_from_meta()`
- [ ] Codex subagent-activity relay (PR #93) intact
- [ ] `OnboardingUpsell::set_dismissed(true, cx)` still in the `ThreadDisplayNotification` handler
- [ ] Built-in agent hiding still under `cfg(not(feature = "external_websocket_sync"))`
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved (streaming `message_added`)
- [ ] Do NOT edit `show_sign_in` / `trust_all_worktrees` in `default.json` — audit the cfg gates in `title_bar.rs` and `trusted_worktrees.rs` instead
- [ ] Do NOT try to preserve `NativeAgentSessionList` — it no longer exists
- [ ] `title_bar`'s `external_websocket_sync` dep stays `optional = true`; `rust-embed` keeps `debug-embed`; `wait_for_tools_ready` uses `cx.background_executor().timer()`
- [ ] Migration banner `Hidden`; trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches exhaustive; Fix 1b cfg-gated `return;` is still the FIRST statement of its `BaseView::Uninitialized` branch

## Phase 9 — Build and test (hard gates)

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — zero errors
- [ ] Feature-off `cargo check -p zed` via a one-off Docker run in the build image
- [ ] `cargo test -p external_websocket_sync` — full pass (covers the elicitation relay)
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] `cargo test -p agent_servers qwen_permission_answers_serialize_at_the_response_top_level` (agent-questions wire format)
- [ ] `cargo test -p agent_servers` session-lifecycle tests pass against the new release-observer model
- [ ] `go mod tidy` in `crates/external_websocket_sync/e2e-test/helix-ws-test-server/`
- [ ] E2E HARD GATE: `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` — all 17 phases green for `zed-agent`
- [ ] E2E with `E2E_AGENTS="zed-agent,claude"` — all 17 phases green (one retry allowed for the known Claude Phase-1 npm flake)
- [ ] helixml/zed CI green: `build-zed`, `zed-e2e-image`, `zed-e2e-headless-smoke`, `zed-e2e-headless-plan`, `zed-e2e-protocol-lifecycle`, `zed-e2e-live-claude-latest`

## Phase 10 — Porting guide (written incrementally, NOT at the end)

- [ ] Insert `## Merge 003182 (2026-09-14)` above `## Merge 2026-07-29 …` (~line 743) at the START of the work
- [ ] Window summary: 616 commits, 47-day window, fence `b9256fa8f0` → upstream HEAD, ACP `2.0.0 → 2.1.0`, rustc `1.95.0 → 1.98.1`, zed `1.15.0 → 1.21.0`
- [ ] `### ACP 2.0.0 → 2.1.0` — what broke, the struct-literal sweep result, whether the `JsonRpc*` derives survived
- [ ] `### ACP session lifecycle: ref counting → observe_release` — how `force_close_session` was re-expressed
- [ ] `### Re-hosting the agent-questions relay` — `RawSessionNotification`, `RawRequestPermissionResponse`, the Codex `jetbrains` meta
- [ ] `### Conflicts and Resolutions` — all eight files, hunk by hunk
- [ ] `### git_ui → git_ui_core migration` — the full moved/stayed symbol map
- [ ] `### rustc 1.95 → 1.98` — bump, Docker-builder outcome, fork diagnostics
- [ ] `### Crate churn` — six added (incl. `lsp_command_selector`), five removed
- [ ] `### Retired / superseded Helix patches` — constraints #5, #8, #10, #12
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Extend the commit-history table; correct stale Rebase-Checklist entries
- [ ] Note that 002701, 002930, 003012 and 003081 were all planned and never executed

## Phase 11 — Land

- [ ] Re-fetch `upstream/main` and `origin/main`; run an extension round if upstream advanced materially
- [ ] Push `feature/003182-merge-latest-zed` to `origin`; do not force-push `main`; leave `origin/helix-fork` untouched
- [ ] Bump `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` to the post-merge SHA on `feature/003182-merge-latest-zed`
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` into this task directory
- [ ] Report: commits merged, conflicts resolved, tests run with results, and anything left undone
