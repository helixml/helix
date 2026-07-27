# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork

## 0. Read the playbooks first

- [ ] Read `helix-specs/design/tasks/002265_merge-latest-zed/{requirements,design,tasks}.md`
- [ ] Read `helix-specs/design/tasks/002251_merge-latest-zed/…` (ACP-bump guidance)
- [ ] Read `helix-specs/design/tasks/002224_merge-latest-zed/…` incl. its
      `pull_request_zed.md` / `pull_request_helix.md` as PR-doc templates
- [ ] Read `/home/retro/work/zed/portingguide.md` in full — especially
      §"Critical Fixes" (line 222) and §"Rebase Checklist" (lines 488–542)
- [ ] Note that 002224/002251/002265 **never landed**; this merge subsumes all three

## 1. Measure the baseline

- [ ] `git fetch upstream --no-tags && git fetch origin`; confirm `upstream` =
      `https://github.com/zed-industries/zed.git`
- [ ] `git checkout main && git pull origin main`
- [ ] Record fence SHA, `main..upstream/main` count, upstream HEAD SHA + date
      (planning values: `e45e42af6e`, 709, `50da8c40dc` 2026-07-26)
- [ ] Confirm ACP delta: fork `=0.14.0` → upstream `=2.0.0`
- [ ] Enumerate conflicts up front:
      `git merge-tree --write-tree --name-only main upstream/main`
- [ ] Enumerate the fork-only commits that must survive (PRs #62–#71)

## 2. Start the branch and the porting-guide entry

- [ ] `git checkout -b feature/002353-merge-latest-zed`
- [ ] Insert `## Merge 002353 (2026-07-27)` at the top of `portingguide.md`'s
      merge-history list with the measured window summary — **before** merging
- [ ] `git merge upstream/main`

## 3. Resolve the 9 textual conflicts (porting-guide bullet per resolution)

- [ ] `.github/workflows/hotfix-review-monitor.yml` — `git rm` (keep fork deletion)
- [ ] `.github/workflows/stale-pr-reminder.yml` — `git rm`
- [ ] `.gitignore` — keep both sides
- [ ] `crates/acp_thread/src/acp_thread.rs` — keep upstream's `is_same_turn` /
      `StatusChanged` emissions **and** Helix's `stopped_emitted_for_task` guards
      (Critical Fixes #8/#9), on both the tx-dropped and normal-completion paths
- [ ] `crates/agent_servers/src/acp.rs` — keep upstream's
      `client_capabilities_for_agent()` **and** Helix's `SessionCreationGuard` +
      `session_creation_chain` slot-guard wrapper around upstream's new
      `send_request(..).block_task()` call shape
- [ ] `crates/agent_ui/src/agent_panel.rs` — re-add the cfg-gated
      `external_websocket_sync_dep` and `tokio::sync::mpsc` imports upstream's
      import-block removal took out
- [ ] `crates/agent_ui/src/config_options.rs` — re-base `current_model_value()`
      onto upstream's renamed `first_config_option_id_matching`
- [ ] `crates/language_models/src/provider/open_ai.rs` — merge upstream's
      `ChatCompletionMaxTokensParameter::MaxCompletionTokens, None` args with
      Helix's `chat_completions_reasoning_effort(&self.model)`
- [ ] `crates/reqwest_client/src/reqwest_client.rs` — keep upstream's
      keepalive/pool settings **and** Helix's `ZED_HTTP_INSECURE_TLS` branch
- [ ] `Cargo.toml`: take upstream's `agent-client-protocol = "=2.0.0"`; keep
      Helix workspace members and `rust-embed`'s `debug-embed`
- [ ] `Cargo.lock`: `git checkout --theirs`

## 4. Remove dead code

- [ ] Delete `crates/agent_ui/src/acp.rs` and `crates/agent_ui/src/acp/`
      (not declared in `agent_ui.rs`; deleted upstream in #50201) — record the
      deletion in the porting guide's "Retired Helix patches" section
- [ ] Correct `portingguide.md` Rebase-Checklist items #3 and #8 to say
      `conversation_view.rs` instead of `thread_view.rs`

## 5. ACP 0.14.0 → 2.0.0 repair loop

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev`; work the error list
- [ ] Convert `non_exhaustive` struct literals to builders (use upstream's
      `client_capabilities_for_agent()` as the idiom reference)
- [ ] Check `acp::ErrorCode` match arms for new variants
- [ ] Verify `AgentConnection` impls still satisfy the grown trait
      (`resume_session`, `close_session`, `request_elicitations`,
      `session_config_options`, `session_list`, `client_user_message_ids`, `retry`)
- [ ] Verify `thread_service.rs` call sites still compile — `new_session` /
      `load_session` signatures were unchanged, so expect few or no edits
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` → 0
      (including `#[cfg(test)]` code)
- [ ] Keep `BaseView` / `ContextServerStatus` / new ACP status matches exhaustive
- [ ] Append each repaired call site to the porting guide as it is fixed
- [ ] Escalate to the user on any ACP change whose semantics are unclear

## 6. Audit the auto-merged files (auto-merged ≠ correct)

- [ ] Run the full grep block in `design.md` §"Audit auto-merged files"
- [ ] `agent_panel.rs` — Fix 1b draft-suppression `return;` is still the FIRST
      statement of its `BaseView::Uninitialized` branch
- [ ] `agent_panel.rs` — `ThreadDisplayNotification` handler still calls
      `OnboardingUpsell::set_dismissed(true, cx)` and initialises
      `NativeAgentSessionList`; Fix #11 entity-identity guard intact
- [ ] `agent_panel.rs` — `request_permission()` auto-approve, agent_type
      serialization, `send_agent_ready`, `acp_history_store()` intact
- [ ] `conversation_view.rs` — `from_existing_thread()` field set matches its
      `::new` sibling; THREAD_REGISTRY register/unregister, thread-load lock,
      `ensure_thread_subscription`, `send_agent_ready` intact
- [ ] `conversation_view/thread_view.rs` — `current_model_id()` three-way fallback
- [ ] `acp_thread.rs` — Critical Fixes #3 (`content_only`), #5 (stale pending
      flush), #6/#9 (`stopped_emitted_for_task`), #8 (`drop(turn.send_task)`)
- [ ] `agent/src/agent.rs` — Fix #1 `pending_sessions`; no `smol::Timer`
- [ ] `zed/src/zed.rs` — `initialize_agent_panel` + WebSocket init intact
- [ ] `zed/src/main.rs` — `--allow-multiple-instances`, `--headless` (all three
      call sites), `initialize_headless()`, `build_application(headless)`
- [ ] `thread_service.rs` — windowless `cx.subscribe()` preserved (incremental
      `message_added` streaming); turn-scoped `request_id` logic intact
- [ ] `feature_flags/src/flags.rs` — `AcpBetaFeatureFlag::enabled_for_all() -> true`
- [ ] `assets/settings/default.json` — `trust_all_worktrees: true`,
      `show_sign_in: false`, branding/onboarding settings
- [ ] `extensions_ui.rs` — 3× `// HELIX: External agent` markers
- [ ] Built-in agent hiding still under `cfg(not(feature = "external_websocket_sync"))`
- [ ] `title_bar` dep `optional = true` + `render_restricted_mode`
- [ ] `grep_tool.rs` — `truncate_long_lines()` / `MAX_LINE_CHARS = 500`
- [ ] `dev_container_suggest.rs` early return; migration banner `Hidden`;
      trial-end upsell early return
- [ ] Walk all 44 numbered items of `portingguide.md` §"Rebase Checklist"

## 7. Build gates

- [ ] `./stack build-zed dev` — zero errors (feature ON)
- [ ] Feature-OFF check: `cargo check -p zed` without `external_websocket_sync`
      via a one-off Docker run in the build image
- [ ] Confirm `Cargo.lock` regenerated cleanly and committed

## 8. Tests

- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send` (Critical Fix #6)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized`
- [ ] `(cd crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy)`
- [ ] Copy the fresh binary to `e2e-test/zed-binary`
- [ ] `./run_docker_e2e.sh` — all 16 phases green for `zed-agent`
- [ ] `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` — all 16 phases green
      for both agents (one retry allowed for the known Claude Phase-1 flake)
- [ ] Confirm UI-state queries return correct `thread_id`, `entry_count`,
      `active_view` (Phase 6)
- [ ] Never use `--no-build` when diagnosing an E2E failure

## 9. Finish the porting guide

- [ ] `### Conflicts and Resolutions` — one subsection per manual conflict
- [ ] `### ACP 0.14 → 2.0 — repaired sites` — one bullet per fixed call site
- [ ] `### Retired Helix patches` — incl. the dead `agent_ui/src/acp/` deletion,
      or explicit "none" for other categories
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Commit-history table extended
- [ ] Note that 002224/002251/002265 never landed and this merge subsumes them
- [ ] Add a Rebase-Checklist entry only for genuinely new recurring gotchas

## 10. Push and hand off

- [ ] Re-fetch `upstream/main` and `origin/main`; absorb any out-of-band pushes;
      run an extension round if upstream advanced materially during the work
- [ ] `git push -u origin feature/002353-merge-latest-zed`
- [ ] Bump `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` to the
      new merge HEAD on branch `feature/002353-merge-latest-zed`; push
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` into
      `helix-specs/design/tasks/002353_merge-latest-zed/`
- [ ] Confirm the helixml/zed CI pipeline is green
- [ ] Do **not** force-push `main`; do **not** open PRs from the agent
