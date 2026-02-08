# Zed Fork Rebase to Fresh Upstream

**Date:** 2026-02-07
**Status:** Port complete. All fixes applied. Built-in agents disabled. Qwen Code ACP verified working. ACP beta features enabled. Remaining: rebuild Zed, test session resume end-to-end.

## Summary

Ported all Helix-specific Zed changes from the old fork (`~/pm/zed`, branch `main` on `helixml/zed`) to a fresh upstream Zed checkout (`~/pm/zed-upstream`, branch `helix-fork`). The goal was to eliminate accumulated merge debt and reduce the fork surface area.

## Key Locations

- **Old fork:** `/prod/home/luke/pm/zed` (origin: `git@github.com:helixml/zed`, branch `main`)
- **New fork:** `/prod/home/luke/pm/zed-upstream` (branch `helix-fork`, pushed to `helixml/zed`)
  - Remotes: `origin` = `https://github.com/zed-industries/zed.git` (upstream, read-only), `helix` = `git@github.com:helixml/zed.git` (push via SSH)
  - Push with: `git push helix helix-fork`
- **GitHub:** `helixml/zed` has both `main` (old fork) and `helix-fork` (new clean port) branches
- **Helix repo:** `/prod/home/luke/pm/helix` (this repo)
- **E2E test files:** Both `~/pm/zed/crates/external_websocket_sync/e2e-test/` and `~/pm/zed-upstream/crates/external_websocket_sync/e2e-test/`

## Commits on helix-fork branch

1. `4cae6d9` - Port Helix fork changes to fresh upstream Zed
2. `54296a7` - Add WebSocket protocol spec, mock server, and test infrastructure
3. `b063ae0` - Add E2E test infrastructure with Docker container
4. `463b1cc` - Fix E2E test infrastructure: Docker caching, headless Zed startup
5. `bc52393` - Fix model configuration race and E2E test settings
6. `5fe75be` - Fix WebSocket event forwarding for thread_service-created threads
7. `746a9c4` - Add multi-thread E2E test: follow-ups and thread transitions

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

**Model configuration race (FIXED)**
- Root cause: `NativeAgentConnection::new_thread()` checks `registry.default_model()` which returns `None` because provider authentication hadn't completed
- Two sub-issues found and fixed:
  1. **Auth timing**: Providers need `authenticate()` called before `NativeAgent::new()` (which calls `refresh_list()`). Fixed by pre-authenticating all providers in `create_new_thread_sync()` before `server.connect()`
  2. **Model ID mismatch**: Settings used `claude-sonnet-4-5` but Zed's internal ID is `claude-sonnet-4-5-latest` (with `-latest` suffix). Fixed in `run_e2e.sh`
- Also needed: `ca-certificates` package in runtime Docker stage for TLS API calls
- Also needed: `language_models.anthropic.api_url` in settings.json (same pattern as Helix's `zed_config.go`)
- Fix approach used: Pre-authenticate providers + explicitly call `select_default_model()` in thread_service.rs before creating the NativeAgent connection

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

## Post-Rebase Thread Sync Fix (2026-02-08)

### Problem: Thread auto-open and live updates broken after rebase

After force-pushing the rebased fork to `helixml/zed` main, three issues appeared:

1. **Agent sidebar doesn't open on active thread** — The `ThreadDisplayNotification` handler in the rebased code only subscribed to `Stopped` events for WebSocket forwarding, never displayed the thread in the UI
2. **Frozen snapshot when clicking thread** — No `AcpServerView` wrapping the thread entity meant no event subscriptions for UI rendering
3. **WebSocket sync to Helix regressed** — Only `Stopped` events were forwarded; `NewEntry`, `EntryUpdated`, `TitleUpdated` were lost
4. **Restricted mode enabled by default** — Upstream Zed's `trust_all_worktrees: false` default blocked MCP servers and project settings

### Root cause: Architectural mismatch

The old fork had `AcpThreadView::from_existing_thread()` (~170 lines) that directly wrapped an existing `Entity<AcpThread>` into a view. The rebased fork restructured the UI — `AcpThreadView` is now always wrapped inside `AcpServerView` (which manages connection state). The `from_existing_thread` method was dropped during the port because `AcpServerView` normally requires going through `agent.connect()` → `initial_state()` → async loading → `ServerState::Connected`.

**Why `open_thread()` doesn't work:** `open_thread()` creates a NEW `AcpServerView` which calls `agent.connect()` creating a new `NativeAgent` instance. The thread created by `thread_service` (WebSocket handler) lives in a DIFFERENT `NativeAgent` instance. The new connection can't find it via `load_session()` or `resume_session()`. This is exactly the problem the old fork's `from_existing_thread` solved.

### Fix: `AcpServerView::from_existing_thread()` (commit `e0cc99f`)

Added a new constructor that bypasses the connection/loading path:

**`crates/agent_ui/src/acp/thread_view.rs`:**
- `HeadlessConnection` — No-op `AgentConnection` impl for `ConnectedServerState`. Returns errors for `new_thread()`, `prompt()`, etc. since headless threads are driven by WebSocket, not user input.
- `AcpServerView::from_existing_thread()` — Takes existing `Entity<AcpThread>`, creates `EntryViewState`, syncs existing entries into `ListState`, subscribes to thread events via `Self::handle_thread_event` (on `AcpServerView` for both UI updates and WebSocket forwarding), creates `AcpThreadView::new()` wrapping the entity, sets `ServerState::Connected` immediately.

**`crates/agent_ui/src/agent_panel.rs`:**
- `ThreadDisplayNotification` handler now:
  1. Focuses the agent panel
  2. Checks for duplicate (already showing this thread)
  3. Creates `AcpServerView::from_existing_thread()` wrapping `notification.thread_entity`
  4. Sets it as active view via `set_active_view(ActiveView::AgentThread { thread_view }, true, ...)`

**`assets/settings/default.json`:**
- Changed `trust_all_worktrees: false` → `true` to disable restricted mode

### What's different from old fork's `from_existing_thread`

| Aspect | Old fork | New fork |
|--------|----------|----------|
| **View type** | `AcpThreadView` directly | `AcpServerView` wrapping `AcpThreadView` |
| **ActiveView variant** | `ExternalAgentThread { thread_view: Entity<AcpThreadView> }` | `AgentThread { thread_view: Entity<AcpServerView> }` |
| **Connection** | N/A (view was standalone) | `HeadlessConnection` no-op impl |
| **Title editor** | Created if thread supports titles | `None` (skipped) |
| **Mode/model selectors** | Created from connection | `None` (HeadlessConnection returns None for these) |
| **`AgentDiff::set_active_thread`** | Called | NOT called (potential issue for diff viewer) |
| **Event handler** | `AcpThreadView::handle_thread_event` | `AcpServerView::handle_thread_event` (same events, different struct) |

### Potential issues (NOT YET TESTED)

1. **Missing `AgentDiff::set_active_thread` call** — May affect diff viewer for headless threads
2. **No title/mode/model selectors** — UI will be more limited for headless threads (but old fork had same limitation since connection doesn't support these)
3. **Subscription ownership** — Thread event subscriptions are created on `AcpServerView` context but stored in `AcpThreadView._subscriptions`. This matches the normal path in `initial_state()`, but hasn't been tested.

### Key commits

| Commit | Repo | Description |
|--------|------|-------------|
| `a57d7eb6` | helixml/zed (`old-fork-backup` branch) | Last working old fork version |
| `5fe75bea` | helixml/zed | First rebased version (force-pushed to main) |
| `cf72593a` | helixml/zed | First fix attempt: `open_thread()` approach (BROKEN — creates wrong connection) |
| `e0cc99ff` | helixml/zed | Proper fix: `from_existing_thread()` on AcpServerView |

### Git locations

- **Old fork backup:** `helixml/zed` branch `old-fork-backup` (commit `a57d7eb6`)
- **Current main:** `helixml/zed` branch `main` (commit `e0cc99ff`)
- **Local repos:** `/prod/home/luke/pm/zed` (main, has both old and new history), `/prod/home/luke/pm/zed-upstream` (helix-fork branch)

## Other Fixes in This Session (2026-02-08)

### Helix API: PROVIDERS_MANAGEMENT_ENABLED crash
- **File:** `docker-compose.dev.yaml` line 82
- **Problem:** `${PROVIDERS_MANAGEMENT_ENABLED:-}` resolves to empty string, Go's `envconfig` can't parse `""` as bool
- **Fix:** Changed to `${PROVIDERS_MANAGEMENT_ENABLED:-true}` to match Go config default

### Helix API: Git sync auto-force-update for force-pushed upstreams
- **File:** `api/pkg/services/git_repository_service_pull.go`
- **Problem:** `SyncBaseBranch` returned `BranchDivergenceError` when `ahead > 0`, blocking tasks when upstream was force-pushed (e.g., the Zed repo rebase)
- **Fix:** Base branch is pull-only, so local-ahead commits are always stale. Changed to force-update local to match upstream with a warning log instead of an error.

### Helix CI: Zed build with `--locked` flag
- **File:** `.drone.yml` line 1584
- **Problem:** Rust 1.93 tries to update `Cargo.lock` but Zed source is mounted read-only (`:ro`) in CI
- **Fix:** Added `--locked` to `cargo build` command. Also fixed `Cargo.lock` in helixml/zed (missing `agent_settings` dependency).

### Frontend: Thread dropdown for spec task chat panel
- **Files:** `frontend/src/services/specTaskService.ts`, `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
- **Feature:** Dropdown selector in chat panel header to switch between Zed threads (when user creates new threads due to context limits)
- **Labels:** "Main thread" for `planning_session_id`, additional threads by name or "Thread N"
- **PR:** #1596 on helixml/helix

## E2E UI State Query (2026-02-08)

### Problem
The existing E2E test only validates WebSocket protocol events (`thread_created`, `message_completed`). These events come from `thread_service.rs` which works independently of the UI layer. If `from_existing_thread` is broken, the protocol events still flow correctly but the agent panel never displays the thread — a silent regression.

### Solution: `query_ui_state` command (commit `a83ddc0`)
Added a new WebSocket command that lets the mock server query the agent panel's actual UI state:

- **Mock sends:** `{"type": "query_ui_state", "data": {"query_id": "q1"}}`
- **Zed responds:** `{"event_type": "ui_state_response", "data": {"query_id": "q1", "active_view": "agent_thread", "thread_id": "...", "entry_count": 5}}`

The mock server queries after each phase:
1. After Phase 1 completion → verify `active_view == "agent_thread"`, correct `thread_id`, `entry_count >= 2`
2. After Phase 2 completion → verify same `thread_id` (follow-up), `entry_count` increased
3. After Phase 3 completion → verify DIFFERENT `thread_id` (new thread switched)

### Architecture
Uses the same global callback pattern as `ThreadDisplayNotification`:
- `external_websocket_sync.rs`: `GLOBAL_UI_STATE_QUERY_CALLBACK` + `request_ui_state_query()`
- `agent_panel.rs`: callback handler inspects `self.active_view`, reads thread state, sends `UiStateResponse`
- `websocket_sync.rs`: `query_ui_state` command handler dispatches to callback

### Python concurrency fix
The mock server message loop (`async for message in websocket`) is sequential. Sending `query_ui_state` and waiting for the response in the same iteration would deadlock. Fixed by spawning phase advancement as `asyncio.create_task()` so the message loop stays unblocked to receive `ui_state_response`.

### Go handler compatibility
Verified the Helix Go handler (`websocket_external_agent_sync.go`) has a `default` case in the event type switch that logs a warning and returns `nil`. The `ui_state_response` events are safely ignored in production.

## WebSocket Event Flow Fix (2026-02-08, commit `cc037db`)

### Root Cause: GPUI subscribe_in vs cx.spawn() window context

The rebased fork's NativeAgentConnection bridge emits `AcpThreadEvent` from `cx.spawn()` tasks that lack a window context. GPUI's `subscribe_in` wraps callbacks in `window_handle.update()` which silently fails when no window context is available. This means assistant entry events emitted by the bridge never reach the `AcpServerView::handle_thread_event` subscriber.

**Old fork architecture (working):**
```
thread_service → AcpThread.send() → NativeAgentConnection bridge
                                      ↓ cx.spawn() (no window)
                                    AcpThread.push_assistant_content_block()
                                      ↓ cx.emit(NewEntry)
                                    AcpThreadView.handle_thread_event [subscribe_in]
                                      ↓ (WORKED because old fork had different
                                         subscription ownership pattern)
                                    send_websocket_event(MessageAdded)
```

**New fork architecture (broken, then fixed):**
```
thread_service → AcpThread.send() → NativeAgentConnection bridge
                                      ↓ cx.spawn() (no window)
                                    AcpThread.push_assistant_content_block()
                                      ↓ cx.emit(NewEntry)
                                    AcpServerView.handle_thread_event [subscribe_in]
                                      ✗ SILENTLY DROPPED (no window context)

FIX: thread_service sends WebSocket events directly after send completes:
thread_service → AcpThread.send().await → read entries → send_websocket_event()
```

### Verification
- Old fork E2E test: PASSES (LLM responds, events flow)
- New fork E2E test before fix: FAILS (LLM responds, events never reach WebSocket)
- New fork E2E test after fix: Protocol events PASS (message_added, message_completed for all 3 phases)
- UI state queries partially pass (active_view=agent_thread works, but thread_id mismatch and entry_count=0 for phases 1-2 due to subscribe_in still not delivering bridge events to UI)

### UI Freeze Fix (commit `55882e8`)

Two fixes for the frozen thread display:

1. **Channel-based event forwarding**: Use windowless `cx.subscribe` (on `Context<AcpServerView>`) to catch ALL thread events including bridge events. Forward through `tokio::sync::mpsc::unbounded_channel<()>` to a `cx.spawn_in(window, ...)` task that has window context. Inside the task, sync entries to `entry_view_state` and `list_state`. Key: `synced_count` tracks already-synced entries to avoid duplicates.

2. **Focus panel ordering**: Moved `workspace.focus_panel::<AgentPanel>()` AFTER `set_active_view()` in the `ThreadDisplayNotification` handler. Previously, `focus_panel` triggered `set_active(true)` which saw `Uninitialized` and called `new_agent_thread()` creating a spurious default thread. Then `set_active_view` replaced it. The UI state query between these two operations returned the wrong thread_id.

**Key code pattern** (`crates/agent_ui/src/acp/thread_view.rs:444-490`):
```rust
// Windowless subscribe catches bridge events
let (event_tx, mut event_rx) = tokio::sync::mpsc::unbounded_channel::<()>();
cx.subscribe(&thread, move |_this, _thread, _event, _cx| {
    let _ = event_tx.send(());
});

// Window-context task processes events
cx.spawn_in(window, async move |this, cx| {
    let mut synced_count = initial_entry_count;
    while event_rx.recv().await.is_some() {
        while event_rx.try_recv().is_ok() {} // batch
        this.update_in(cx, |this, window, cx| {
            // sync entries synced_count..total to entry_view_state + list_state
        }).ok();
    }
}).detach();
```

**Agent panel handler** (`crates/agent_ui/src/agent_panel.rs:667-719`):
```
// CORRECT ORDER:
this.update_in(cx, ...) → from_existing_thread → set_active_view  // FIRST
workspace.focus_panel::<AgentPanel>()                               // SECOND
```

### E2E Test Results After Both Fixes
All 3 phases pass protocol + UI state:
- Phase 1: `thread_id` correct, `entry_count=2` ✅
- Phase 2: same `thread_id`, `entry_count=4` ✅
- Phase 3: different `thread_id`, `entry_count=2` ✅

## Known Issues (2026-02-08, post UI freeze fix)

### 1. Thread NOT Persisted to ThreadStore ("View All" doesn't show it)
The `from_existing_thread` method creates an `AcpServerView` wrapping the headless thread, but does NOT register it in the `ThreadStore` (history). In the old flow, `new_agent_thread()` → `open_thread()` → `push_recently_opened_entry()` handled this. Since we moved `focus_panel` AFTER `set_active_view`, the `new_agent_thread` path is no longer triggered (because `set_active` sees `AgentThread` not `Uninitialized`).

**Fix needed**: Explicitly register the thread in `ThreadStore` in `from_existing_thread` or in the `ThreadDisplayNotification` handler. Look for `push_recently_opened_entry` or `AcpThreadHistory::push_recently_opened_entry` patterns.

### 2. WebSocket Updates Only at Completion (Not Streaming/Incremental)
The `thread_service.rs` fix sends `message_added` + `message_completed` AFTER `send_task.await` completes. This means ALL assistant content arrives in a single `message_added` event at the end, not incrementally. The old fork sent `message_added` after each `NewEntry`/`EntryUpdated` event via `handle_thread_event`.

**Impact**: The Helix session text box shows no progress until the entire LLM turn finishes. Previously it updated after each tool call.

**Fix needed**: Add intermediate `message_added` WebSocket events during the LLM response. Options:
1. Have the channel-based event forwarder in `from_existing_thread` also send WebSocket events
2. Subscribe to AcpThread events in `thread_service.rs` and forward them incrementally
3. Use the `App::subscribe` pattern (fires without window context) specifically for WebSocket forwarding

Option 2 is cleanest: `thread_service.rs` already has the thread entity. Add a GPUI `App::subscribe` on the AcpThread to send `message_added` events as entries arrive, and send `message_completed` at the end.

**Key architectural insight**: `App::subscribe` (not `subscribe_in`) DOES fire for events from `cx.spawn()` — we confirmed this in testing. It just can't call `handle_thread_event` because that needs window context. But `send_websocket_event` does NOT need window context, so `App::subscribe` is perfect for WebSocket forwarding.

### 3. "Welcome to Zed AI" Onboarding Still Shows
The E2E settings set `"show_onboarding": false` but the onboarding/upsell screen still appears. Need to check:
- `AgentPanelOnboarding` / `OnboardingUpsell` in `agent_ui`
- Whether `show_onboarding` setting name changed in the rebase
- Whether additional settings are needed (e.g., `OnboardingUpsell::set_dismissed(true, cx)`)

## Commits on helixml/zed main (latest first)

| Commit | Description |
|--------|-------------|
| `55882e8` | fix: UI freeze fix (channel forwarding + focus_panel ordering) |
| `cc037db` | fix: WebSocket events from thread_service instead of UI subscription |
| `a83ddc0` | feat: query_ui_state command for E2E UI verification |
| `e0cc99f` | fix: implement from_existing_thread for AcpServerView |
| `cf72593` | fix: restore thread auto-open and disable restricted mode |

## Commits on helixml/helix feature/multi-thread-dropdown (latest first)

| Commit | Description |
|--------|-------------|
| `008f481` | fix: update zed to 55882e8 with UI freeze fix |
| `917d22f` | fix: update zed to cc037db with WebSocket event flow fix |
| `b543f01` | feat: add UI state query to E2E test, update zed commit |
| `63f96c5` | docs: update design doc with post-rebase thread sync fix context |

## Key Architecture Knowledge

### GPUI Subscription Behavior
- `cx.subscribe_in(&entity, window, handler)` — only fires when event is emitted within a window context. Bridge events from `cx.spawn()` are SILENTLY DROPPED.
- `cx.subscribe(&entity, handler)` on `Context<T>` — fires regardless of context, but callback doesn't get `&mut Window`
- `App::subscribe(&entity, handler)` — fires regardless of context, callback gets `&mut App` (can call `send_websocket_event` but not UI methods)
- Solution for UI: windowless subscribe → channel → `cx.spawn_in(window)` for window context
- Solution for WebSocket: `App::subscribe` directly

### Thread Creation Flow (NativeAgent)
```
NativeAgentConnection::new_thread()
  → NativeAgent::new_session()
    → Thread::new() — creates agent::Thread
    → register_session() — creates AcpThread, stores both in sessions HashMap
  → Returns Entity<AcpThread>

NativeAgentConnection::prompt()
  → run_turn() — gets (thread, acp_thread) from sessions HashMap
    → thread.send() — calls LLM, returns event receiver
    → handle_thread_events() — spawns cx.spawn() that forwards ThreadEvent → AcpThread
```

### Event Bridge (agent.rs:993-1086)
```
ThreadEvent::AgentText → acp_thread.push_assistant_content_block() → cx.emit(NewEntry)
ThreadEvent::Stop → return Ok(PromptResponse)
```

### Entry Sync Pattern (for UI list rendering)
```rust
entry_view_state.update(cx, |view_state, cx| {
    view_state.sync_entry(index, &thread, window, cx);  // needs Window
});
list_state.splice_focusable(range, focus_handles);
```

### AcpThreadHistory / ThreadStore
- `push_recently_opened_entry(HistoryEntryId::AcpThread(session_id), cx)` — adds to history
- Used by "View All" to show thread list
- Called in normal `open_thread` flow but NOT in `from_existing_thread`

## Streaming WebSocket Updates Fix (2026-02-08, commit `01c0c11`)

### Problem
WebSocket `message_added` events only arrived AFTER the entire LLM turn completed. The Helix session chat showed no progress during inference.

### Fix
Added `cx.subscribe()` (windowless) in `thread_service.rs` BEFORE `thread.send()` in both `create_new_thread_sync` and `handle_follow_up_message`. The subscription catches `NewEntry` and `EntryUpdated` events during streaming and sends `message_added` events incrementally. A final cumulative `message_added` + `message_completed` is still sent after `send_task.await` as a safety net.

Key insight: `cx.subscribe()` (on `Context<T>`, not `subscribe_in`) fires for events emitted from `cx.spawn()` without window context. `send_websocket_event()` doesn't need window context.

### Thread Persistence Fix (same commit)
Added `NativeAgentSessionList::new(thread_store, cx)` in the `ThreadDisplayNotification` handler to set up `acp_history.set_session_list()`. Made `NativeAgentSessionList::new()` public in `agent.rs`.

### Onboarding Fix (same commit)
Added `OnboardingUpsell::set_dismissed(true, cx)` in the `ThreadDisplayNotification` handler to prevent "Welcome to Zed AI" overlay on external threads.

### E2E Streaming Validation
Updated `run_e2e.sh` mock server to validate that `message_added` events arrive BEFORE `message_completed` in each phase.

## Multi-Thread Dropdown Bug Fix (2026-02-08)

### Problem
The thread dropdown in the Helix spec task chat UI never appeared. Zero `spec_task_zed_threads` records existed in the database despite multiple threads being created in Zed.

### Root Cause
`SpecTaskWorkSession.HelixSessionID` had a `uniqueIndex` constraint (`gorm:"not null;size:255;uniqueIndex"`). Since all Zed threads for a spec task share the same planning session, the second call to `CreateSpecTaskWorkSession` with the same `HelixSessionID` failed with a unique constraint violation. The error was logged in a background goroutine and silently swallowed.

### Fix
Changed `uniqueIndex` to `index` in `api/pkg/types/spec_task_multi_session.go`:
```go
// Before:
HelixSessionID string `json:"helix_session_id" gorm:"not null;size:255;uniqueIndex"`
// After:
HelixSessionID string `json:"helix_session_id" gorm:"not null;size:255;index"`
```
Also dropped and recreated the PostgreSQL index to remove the unique constraint.

### ACP Protocol Compatibility Analysis (2026-02-08)

Analyzed compatibility between Zed's `agent-client-protocol = "0.9.4"` (schema crate v0.10.8) and Qwen Code's ACP implementation.

**Result: COMPATIBLE** — Both sides use camelCase JSON serialization:
- Zed's schema crate uses `#[serde(rename_all = "camelCase")]` on all ACP types
- Qwen Code uses camelCase field names in TypeScript
- All JSON-RPC method names match exactly
- Session list, new session, load session, prompt, cancel all compatible
- MCP server configuration compatible

### Built-in Agent Disabling

Three built-in agents (Claude Code, Codex, Gemini CLI) appear as hardcoded menu items in `agent_panel.rs:2490-2584`. They are NOT filtered by the existing `external_agents()` filter (that filter at lines 2585-2595 only removes them from the CUSTOM agents list to avoid duplication).

**Fixed** in commit `3ae2f1e`: Wrapped the three hardcoded `.item()` blocks with `#[cfg(not(feature = "external_websocket_sync"))]` using `.map()` pattern.

### ACP Beta Feature Flag (2026-02-08)

The ACP session management features (list, load, resume) are gated behind `AcpBetaFeatureFlag` in Zed. This flag uses the default `FeatureFlag` trait: `enabled_for_staff() = true`, `enabled_for_all() = false`. In release builds (which our `./stack build-zed` produces), `cfg!(debug_assertions)` is false, so the flag evaluates to false — blocking all session management features.

**Fix** (commit `4e87001`): Override `enabled_for_all()` to return `true` in `feature_flags/src/flags.rs`. This enables session list/load/resume for all users in all build modes.

### Qwen Code End-to-End Verification (2026-02-08)

Verified Qwen Code works end-to-end with the rebased Zed:
- **Settings-sync-daemon**: Correctly generates `agent_servers.qwen` with `"type": "custom"` (required by rebased Zed's tagged enum deserialization)
- **ACP protocol**: Qwen Code v1 with camelCase JSON is fully compatible with Zed's schema v0.10.8 (both use `#[serde(rename_all = "camelCase")]`)
- **Session capabilities**: Qwen advertises `loadSession: true`, `sessionCapabilities.list: {}` — Zed's new `resume` field is `Option<T>` and safely absent
- **End-to-end test**: Session `ses_01kgyrs0dwefcp50rhbe3swv2b` — thread created, planning message processed by Qwen, AI response completed, `message_completed` sent back successfully
- **No code changes needed** in either the Qwen Code fork or the settings-sync-daemon

## Next Steps

1. ~~**Fix model configuration in E2E test**~~ DONE
2. ~~**Get E2E test passing with real LLM inference**~~ DONE
3. ~~**Fix new fork event forwarding**~~ DONE (via `from_existing_thread`)
4. ~~**Add multi-thread E2E test**~~ DONE
5. ~~**Update helixml/zed main**~~ DONE
6. ~~**Fix WebSocket event flow regression**~~ DONE (commit `cc037db`)
6b. ~~**Fix UI live updates**~~ DONE (commit `55882e8` — channel-based forwarding)
7. **Add Qwen Code ACP test** - Test with Qwen Code agent using Together AI
8. **Test session resume** - Kill and restart Zed, verify thread state restored
9. ~~**CI integration**~~ DONE
10. ~~**Helix multi-thread session support**~~ DONE
11. ~~**Add UI state query to E2E test**~~ DONE (commit `a83ddc0`)
12. **Rewrite E2E mock server in Go** — Better concurrency model
13. ~~**Fix thread persistence**~~ DONE (commit `01c0c11` — NativeAgentSessionList in ThreadDisplayNotification handler)
14. ~~**Fix streaming WebSocket updates**~~ DONE (commit `01c0c11` — cx.subscribe in thread_service.rs)
15. ~~**Fix "Welcome to Zed AI" onboarding**~~ DONE (commit `01c0c11` — OnboardingUpsell::set_dismissed)
16. ~~**Fix multi-thread dropdown**~~ DONE — Removed uniqueIndex on SpecTaskWorkSession.HelixSessionID
17. ~~**Disable built-in agents**~~ DONE (commit `3ae2f1e`) — Wrapped Claude Code/Codex/Gemini menu items with `cfg(not(feature = "external_websocket_sync"))`
18. ~~**Qwen Code configuration verified**~~ DONE — No changes needed. Settings-sync-daemon generates correct `agent_servers.qwen` with `"type": "custom"` format. ACP protocol v1 with camelCase JSON is compatible between Zed (schema v0.10.8) and Qwen Code.
19. ~~**Test Qwen Code ACP**~~ DONE — Started qwen_code session (`ses_01kgyrs0dwefcp50rhbe3swv2b`), verified thread created, message sent to Qwen, AI response completed successfully, `message_completed` sent back. Full round-trip works.
20. ~~**Enable ACP beta feature flag**~~ DONE (commit `4e87001`) — `AcpBetaFeatureFlag.enabled_for_all()` returns `true`. Without this, session list/load/resume were gated behind staff-only access in release builds, blocking Qwen session persistence.
21. **Test Qwen session resume** — Rebuild Zed with ACP beta enabled, then: start session → create Qwen thread → kill Zed → restart Zed → verify session list shows previous session → resume session
