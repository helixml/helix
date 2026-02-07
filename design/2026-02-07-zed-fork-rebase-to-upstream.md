# Zed Fork Rebase to Fresh Upstream

**Date:** 2026-02-07
**Status:** Port complete, E2E test container functional, model configuration issue remaining

## Summary

Ported all Helix-specific Zed changes from the old fork (`~/pm/zed`, branch `main` on `helixml/zed`) to a fresh upstream Zed checkout (`~/pm/zed-upstream`, branch `helix-fork`). The goal was to eliminate accumulated merge debt and reduce the fork surface area.

## Key Locations

- **Old fork:** `/prod/home/luke/pm/zed` (origin: `git@github.com:helixml/zed`)
- **New upstream copy:** `/prod/home/luke/pm/zed-upstream` (origin: `https://github.com/zed-industries/zed.git`)
- **Working branch:** `helix-fork` (3 commits on top of upstream `main` at `79f38fe`)
- **Helix repo:** `/prod/home/luke/pm/helix` (this repo)
- **E2E test files:** Both `~/pm/zed/crates/external_websocket_sync/e2e-test/` and `~/pm/zed-upstream/crates/external_websocket_sync/e2e-test/`

## Commits on helix-fork branch

1. `4cae6d9` - Port Helix fork changes to fresh upstream Zed
2. `54296a7` - Add WebSocket protocol spec, mock server, and test infrastructure
3. `b063ae0` - Add E2E test infrastructure with Docker container

## What Was Ported (725 lines across 17 files + new crate)

### 1. External WebSocket Sync Crate
- **Location:** `crates/external_websocket_sync/` (~5,500 lines, copied from old fork)
- **Purpose:** Bidirectional WebSocket sync between Zed and Helix server
- **Protocol:** Documented in `crates/external_websocket_sync/PROTOCOL_SPEC.md`
- **Feature flag:** `external_websocket_sync` on the `zed` crate
- **Integration points:**
  - `crates/zed/src/main.rs` - `external_websocket_sync::init(cx)` (cfg-gated)
  - `crates/zed/src/zed.rs` - `setup_thread_handler` + `init_websocket_service` in `initialize_agent_panel` (cfg-gated)
  - `crates/agent_ui/src/agent_panel.rs` - `acp_history_store()`, session serialization (cfg-gated)
  - `crates/agent_ui/src/acp/thread_view.rs` - WebSocket event forwarding on thread events (cfg-gated)
  - `crates/title_bar/src/title_bar.rs` - Connection status indicator
  - `crates/zed/Cargo.toml` - Feature flag: `external_websocket_sync = ["agent_ui/external_websocket_sync", "dep:external_websocket_sync"]`
  - `crates/agent_ui/Cargo.toml` - Feature: `external_websocket_sync = ["external_websocket_sync_dep"]`, optional dep renamed

### 2. Enterprise TLS Skip
- **Files:** `crates/http_client_tls/`, `crates/reqwest_client/`
- **Env var:** `ZED_HTTP_INSECURE_TLS=1` skips certificate verification
- **Use case:** Enterprise deployments with internal CAs

### 3. UI/Branding (Privacy-Focused)
- `assets/settings/default.json` - `show_sign_in: false`, added `show_onboarding`, `auto_open_panel`
- `crates/extensions_ui/` - Removed Claude/Codex/Gemini agent upsell banners
- `crates/title_bar/` - Hidden onboarding banner, added Helix connection indicator
- `crates/agent_settings/` + `crates/settings_content/` - New settings fields

### 4. Qwen Code Shell Output Formatting
- **File:** `crates/acp_thread/src/acp_thread.rs`
- **Function:** `format_shell_output()` - Formats Qwen Code's shell output as markdown tables
- Applied in `ToolCall::to_markdown()` and `markdown_for_raw_output()`

### 5. Multiple Instances Flag
- **File:** `crates/zed/src/main.rs`
- `--allow-multiple-instances` CLI flag bypasses single-instance check

### 6. Workspace Test Fix
- **File:** `crates/workspace/src/persistence.rs`
- Added wildcard arm for `RemoteConnectionOptions::Mock` when `test-support` feature propagates without workspace's own `test-support`

## What Was Dropped (Upstream Caught Up)

| Feature | Why Dropped |
|---------|-------------|
| `crates/acp_runtime/` (local ACP fork) | Upstream uses `agent-client-protocol = "0.9.4"` from crates.io |
| ACP session listing/resume code | Upstream has `list_sessions`, `resume_session`, `supports_load_session` behind `AcpBetaFeatureFlag` |
| Anthropic token counting API rewrite | Upstream's version is sufficient |
| Claude Opus 4.5/4.6 model support | Upstream has all models through 4.6-1m-context |
| Debug crash panic hook | Was debug-only, not needed |

## E2E Test Infrastructure

### Docker Container (`zed-ws-e2e`)

**What works:**
- Docker container builds Zed from source with BuildKit cache mounts (~30s rebuild, ~12min first build)
- Zed starts headlessly using xvfb + lavapipe (software Vulkan) + D-Bus
- WebSocket connection established between Zed and Python mock server
- Full bidirectional protocol flow: `agent_ready`, `chat_message`, `thread_created`
- Screenshots can be captured from xvfb using `import -window root`
- Mock server validates protocol flow and exits with pass/fail

**Build requirements discovered:**
- `libasound2-dev` (ALSA) for build stage
- `libxkbcommon-x11-0` for runtime
- `VK_ICD_FILENAMES=/usr/share/vulkan/icd.d/lvp_icd.json` (not `lvp_icd.x86_64.json`)
- D-Bus session daemon must be running (Zed's GPU init uses ashpd portals for error notifications)
- `ZED_ALLOW_ROOT=true` for Docker root user
- `ZED_ALLOW_EMULATED_GPU=1` to suppress software rendering warning
- `.dockerignore` essential to exclude `target/` (15GB+) and `.git/`

**Blocking issue: Model not selected**
- Screenshot shows Zed with agent panel open, "Select a Model" button visible
- The `settings.json` with `agent.default_model` is written but model isn't applied
- Root cause: `NativeAgentConnection::new_thread()` at `agent.rs:1014-1018` fetches `registry.default_model()` which returns `None` because:
  1. Anthropic provider authentication is async (background task via `authenticate_all_language_model_providers`)
  2. The `update_active_language_model_from_settings()` handler depends on authenticated providers
  3. Thread creation happens before auth completes
- This issue exists in BOTH the old fork and the new fork - it's not a port regression
- In production Helix, this works because Helix's API proxy handles LLM inference (model config on Helix side)
- The Anthropic API key must be set via `ANTHROPIC_API_KEY` env var (Zed reads it from env, not settings.json for security)

**Fix approach needed:**
- Option A: Ensure provider authentication completes before thread creation (await auth task)
- Option B: Add retry logic in thread_service - if model is None, wait and retry
- Option C: Use `handle_models_updated_event()` pattern from `agent.rs:613-636` which sets model on existing threads when it becomes available, but delay `thread.send()` until model is set

### Mock Helix WebSocket Server (Rust, for unit tests)
- **File:** `crates/external_websocket_sync/src/mock_helix_server.rs` (1,737 lines)
- 37 tests passing, 2 ignored (env-dependent)

### E2E Docker Test Files
- **Dockerfile:** `crates/external_websocket_sync/e2e-test/Dockerfile`
  - Two-stage build (builder + runtime) with BuildKit cache mounts
  - Matches Helix's `Dockerfile.zed-build` cache pattern
- **run_e2e.sh:** `crates/external_websocket_sync/e2e-test/run_e2e.sh`
  - Starts D-Bus, xvfb, mock server (Python), Zed
  - Configures Zed via env vars (`ZED_EXTERNAL_SYNC_ENABLED`, `ZED_HELIX_URL`, etc.)
  - Writes `settings.json` with `agent.default_model`
  - Validates protocol: `agent_ready` → `chat_message` → `thread_created` → `message_added` → `message_completed`

### Build & Run Commands
```bash
# Build E2E image (old fork - known working WebSocket code)
cd ~/pm/zed
docker build -t zed-ws-e2e -f crates/external_websocket_sync/e2e-test/Dockerfile .

# Build E2E image (new fork)
cd ~/pm/zed-upstream
docker build -t zed-ws-e2e -f crates/external_websocket_sync/e2e-test/Dockerfile .

# Run test with API key
docker run --rm -e ANTHROPIC_API_KEY=sk-ant-... -e TEST_TIMEOUT=120 zed-ws-e2e

# Take screenshot from running container
docker exec <container> bash -c "DISPLAY=:99 import -window root /tmp/screenshot.png"
docker cp <container>:/tmp/screenshot.png ./screenshot.png
```

## WebSocket Protocol Summary

### Connection
```
ws://{host}/api/v1/external-agents/sync?session_id={id}
Authorization: Bearer {token}
```

### Zed → Helix (SyncMessage)
```json
{"session_id": "...", "event_type": "...", "data": {...}, "timestamp": "..."}
```
Events: `agent_ready`, `thread_created`, `user_created_thread`, `thread_title_changed`, `message_added`, `message_completed`, `thread_load_error`

### Helix → Zed (ExternalAgentCommand)
```json
{"type": "...", "data": {...}}
```
Commands: `chat_message` (with `message`, `request_id`, `acp_thread_id?`, `agent_name?`), `open_thread`

### Readiness Protocol
1. Zed connects → sends `agent_ready` when ACP agent initialized
2. Helix queues commands until `agent_ready`
3. Then sends queued commands

## Native Agent Model Configuration Chain

Traced for debugging the E2E model issue:

1. **Settings load:** `agent_ui::init_language_model_settings()` → watches `SettingsStore` changes
2. **Settings change:** `update_active_language_model_from_settings()` → `registry.select_default_model()`
3. **Provider auth:** `NativeAgent::authenticate_all_language_model_providers()` → background task per provider
4. **Thread creation:** `NativeAgentConnection::new_thread()` → `registry.default_model()` → `Thread::new(model)`
5. **Model update:** `handle_models_updated_event()` → sets model on existing threads if `thread.model().is_none()`
6. **Thread send:** `Thread::send()` → `self.model().context("No language model configured")?`

The race: steps 3-4 can overlap - thread created before auth completes = no model.

## Extension API Assessment

Checked whether Helix changes could be a Zed extension instead of a fork. **Answer: No.**

Zed's extension API (WIT v0.8.0) is WASM-sandboxed for language tooling only:
- Can: LSP, slash commands, context servers, debug adapters, HTTP fetch
- Cannot: WebSocket connections, UI modification, agent panel access, title bar changes, event hooks

The fork approach is the only option for these features.

## Next Steps

1. **Fix model configuration in E2E test** - Either wait for auth, retry, or use handle_models_updated pattern
2. **Get E2E test passing with real LLM inference** - Anthropic API key → model selected → response generated
3. **Add Qwen Code ACP test** - Test with Qwen Code agent using Together AI
4. **Test session resume** - Kill and restart Zed, verify thread state restored
5. **Port E2E fixes to new fork**
6. **Update helixml/zed fork** - Push `helix-fork` branch
7. **Update Helix build scripts** - Point `./stack build-zed` at new fork/branch
8. **CI integration** - Add Docker E2E test to Drone pipeline
