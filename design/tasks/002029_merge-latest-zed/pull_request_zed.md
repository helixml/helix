# Merge upstream Zed (8bdd78e023..1399540715, 261 commits) into Helix fork

## Summary

Brings the Helix fork up to date with upstream Zed `1399540715` (2026-05-21), 261 commits over 10 days since 001996's `8bdd78e023`. All 10 surviving Critical Fixes preserved, all PRs #50/#55/#56/#57 + the `fd26c1a113` Dockerfile.ci fix carried through. Critical Fix #10 (180s context-server timeout) is retired — `e60a1b2789` on fork main reverted it intentionally as a wrong diagnosis. Full E2E (17 phases × 2 agents) green.

## Changes

- **Merge commit `6b39672e5f`**: `git merge upstream/main`. Six conflicts resolved (full reasoning in `portingguide.md` §"Merge 002029"):
  - `.github/workflows/compare_perf.yml` and `release_nightly.yml`: accept upstream (Helix doesn't use Zed's CI).
  - `crates/title_bar/Cargo.toml`: kept Helix `external_websocket_sync` dep, dropped now-unused `feature_flags.workspace = true` (upstream `2e70059cd9` removed its only consumer).
  - `crates/title_bar/src/title_bar.rs` `render_restricted_mode`: kept Helix cfg-gated early-return, adopted upstream's free-function `TrustedWorktrees::has_restricted_worktrees` API.
  - `crates/project/src/agent_server_store.rs` `reregister_agents` destructure: dropped `extension_agents` (removed by `c84c22dab5`), kept Helix's `_subscriptions` / `registry_subscribed`, added trailing `..`.
  - `crates/agent_ui/src/agent_panel.rs` `load_panel` thread-restoration: kept Helix `wait_for_websocket_connected` and `send_agent_ready` in the no-restore branch; adopted upstream `589dc95c87`'s new `thread_to_restore` lookup, `load_agent_thread(thread_id, ...)` call, and `restore_new_draft`.
  - `crates/agent_ui/src/agent_panel.rs` `load_agent_thread` Critical Fix #11: upstream changed the signature from `session_id` to `thread_id`. Adapted the Helix entity-identity guard to look up the session_id via `ThreadMetadataStore::entry(thread_id).session_id` before querying `external_websocket_sync::get_thread`.
  - `crates/agent_ui/src/agent_panel.rs` `ensure_thread_initialized`: kept Helix Fix 1b cfg-gated early-return as the **FIRST statement** of the `BaseView::Uninitialized` branch, before upstream `589dc95c87`'s new `pending_terminal_spawn` / `should_create_terminal_for_new_entry` / ACP-restoration branches.

- **Build fix `edbc05cf99`**: upstream signature drift forced four compile-driven repairs:
  - `agent_servers/acp.rs`: PR #50 chain log-labels now use `directories.cwd.display()` (upstream `c3951af24f` removed the local `cwd` binding; `new_session`/`load_session`/`open_or_create_session` now pass `SessionDirectories` to the rpc_call closure).
  - `agent_ui/conversation_view.rs::from_existing_thread`: hoisted `let root_thread_id = ThreadId::new();` to pass to upstream's new 24-arg `ThreadView::new`; added `vec![]` for the new `available_skills` arg on `SessionCapabilities::new`; initialised new struct fields `last_theme_id: Some(cx.theme().id.clone())`, `draft_prompt_persist_task: None`.
  - `agent_ui/agent_panel.rs` + `zed/main.rs`: added `ContextServerStatus::ClientSecretRequired { .. } => "client_secret_required"` arm to the two Helix UI-state-query loops (upstream MCP work added the variant).
  - Also did the compile-driven `AgentSessionList::supports_delete(&self)` → `supports_delete(&self, &App)` migration across `agent_ui/src/acp/thread_history.rs` (10 references).

- **Re-merge `8841edb2b1`-style** (actually `Merge made by 'ort'` after manual merge): merged `origin/main` once more after our work was done to pick up `e60a1b2789 revert(context_server): DEFAULT_REQUEST_TIMEOUT back to upstream 60s`. Critical Fix #10 retired in `portingguide.md` accordingly.

- **Porting guide updated** (`5da4b2f5f0`, `8692f073b2`): new `## Merge 002029 (2026-05-21)` section with full conflict trail + Pre-existing Breakage Repaired subsections + commit-history extension + retirement of Critical Fix #10.

## Validation

- `./stack build-zed dev`: green, both builds (post-merge + post-revert).
- E2E `zed-agent`: **ALL 17 phases passed**.
- E2E `claude`: **ALL 17 phases passed**, including Phase 17 (live Claude process count == real thread count — the hard gate that Helix PR #56 Fix 1b draft-suppression survived the merge).
- All 11 Critical Fixes accounted for: 10 verified preserved, Fix #10 explicitly retired per `e60a1b2789`.
- Full silent-drift sweep clean (`ActiveView`/`set_active_view`/`draft_threads`/`selected_agent_type` not present; `AcpThreadEvent::Stopped\b([^(]|$)` only matches doc comments; `--allow-multiple-instances` / `--headless` / `debug-embed` / `external_websocket_sync::get_thread` all present; `wait_for_tools_ready` present; no `smol::Timer`).

Release Notes:

- N/A
