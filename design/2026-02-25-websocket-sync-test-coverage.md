# WebSocket Sync Test Coverage Analysis

**Date:** 2026-02-25
**Status:** In Progress
**Branch:** `fix/golden-cache-investigation`

## Overview

This document tracks test coverage for the WebSocket sync protocol between Helix (Go API server) and Zed (Rust IDE). The protocol is bidirectional:

- **Helix → Zed:** `ExternalAgentCommand` messages (`chat_message`, `open_thread`)
- **Zed → Helix:** `SyncMessage` events (`agent_ready`, `thread_created`, `message_added`, `message_completed`, `user_created_thread`, `thread_title_changed`, `thread_load_error`)

## Architecture

```
Helix API Server (Go)                    Zed IDE (Rust)
websocket_external_agent_sync.go    ←→   external_websocket_sync crate
  handleExternalAgentSync()               websocket_sync.rs (WebSocketSync)
  processExternalAgentSyncMessage()       thread_service.rs (ThreadService)
  handleThreadCreated()                   types.rs (SyncEvent enum)
  handleMessageAdded()
  handleMessageCompleted()
  handleAgentReady()
  handleUserCreatedThread()
  handleThreadTitleChanged()
```

### Key In-Memory State (Helix side, lost on restart)
- `contextMappings` — Zed thread ID → Helix session ID
- `sessionToWaitingInteraction` — session ID → interaction ID awaiting response
- `requestToSessionMapping` — request ID → session ID (for correlating thread_created to originating request)
- `externalAgentWSManager` — WebSocket connection manager with readiness protocol

### DB Fallback
When `contextMappings` miss occurs (e.g., after API restart), `findSessionByZedThreadID()` queries the DB via `ListSessions` filtering by `ZedThreadID` metadata.

---

## Test Layers

### 1. Go Unit Tests (MockStore) — DONE

**File:** `api/pkg/server/websocket_external_agent_sync_test.go`
**Run:** `cd api && go test -v -run TestWebSocketSyncSuite -count=1 ./pkg/server/`
**Result:** 46 tests, all passing

These test individual handler methods on `*HelixAPIServer` using gomock `MockStore`. No WebSocket connections — handlers are called directly. This gives precise control over store behavior and edge cases.

| Handler | Tests | Key Scenarios |
|---------|-------|---------------|
| `handleThreadCreated` | 6 | Priority 1 (requestToSessionMapping), Priority 2 (SessionID reuse), Priority 3 (new session), SpectaskLink (copies SpecTaskID), missing thread ID, store error |
| `handleMessageAdded` | 7 (incl. subtests) | First assistant message, same message_id streaming update, new message_id multi-entry, user message, contextMapping miss with DB fallback, missing fields (4 subtests) |
| `handleMessageCompleted` | 5 | Normal completion (state→Complete, preserves reloaded content), no waiting interaction, contextMapping miss with DB fallback, missing thread ID, comment finalization |
| `handleAgentReady` | 3 | Basic readiness, no readiness state, with pending prompt |
| `processPromptQueue` | 3 | No pending, has pending, send fails (marks failed) |
| `processAnyPendingPrompt` | 3 | No pending, has pending, send fails |
| `findSessionByZedThreadID` | 2 | Found, not found |
| `handleUserCreatedThread` | 3 | New session (copies config from agent session), idempotent (already mapped), missing thread ID |
| `linkAgentResponseToComment` | 3 | Comment exists, no comment, empty interaction ID |
| `finalizeCommentResponse` | 3 | Comment exists (clears RequestID/QueuedAt, processes next), no comment, empty request ID |
| `handleThreadTitleChanged` | 2 | Updates session name, no mapped session |
| `processExternalAgentSyncMessage` | 3 | Unknown event type, ping no-op, syncEventHook fires |

### 2. Real Helix + Real Zed E2E (Go test server + Zed binary in Docker) — EXISTS

**Go server:** `~/pm/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go`
**Shell script:** `~/pm/zed/crates/external_websocket_sync/e2e-test/run_e2e.sh`
**Dockerfiles:** `~/pm/zed/crates/external_websocket_sync/e2e-test/Dockerfile` and `Dockerfile.runtime`
**Pre-built binaries:** `e2e-test/zed-binary` (318MB) and `e2e-test/helix-ws-test-server/helix-ws-test-server`

This is the **real E2E test**. A standalone Go binary imports the **actual production** `server.HelixAPIServer` with `memorystore` (in-memory store, no Postgres). It runs 5 sequential test phases against a real Zed binary (with Xvfb for headless rendering).

**Key architecture:**
- Go server uses `server.NewTestServer(store, ps)` — same production handler code
- Registers `srv.ExternalAgentSyncHandler()` on `/api/v1/external-agents/sync`
- Uses `srv.SetSyncEventHook()` to observe all sync events (same hook the Go unit tests use)
- Uses `srv.QueueCommand()` to send commands to the connected Zed agent
- Go module has `replace github.com/helixml/helix => /prod/home/luke/pm/helix` to use local production code
- Zed connects via env vars: `ZED_HELIX_URL=127.0.0.1:$PORT`, `HELIX_SESSION_ID=e2e-test-session-001`

**5 Phases:**

| Phase | Test | What It Validates |
|-------|------|-------------------|
| 1 | Basic thread creation | `chat_message(null)` → agent_ready → thread_created → N×message_added → message_completed |
| 2 | Follow-up on existing thread | `chat_message(acp_thread_id)` → no new thread_created → message_added → message_completed |
| 3 | New thread (context transition) | `chat_message(null)` → new thread_created with different ID → message_completed |
| 4 | Follow-up to non-visible thread | `chat_message(thread_A)` while Thread B is active → message_completed |
| 5 | Simulate user input (Zed→Helix) | `simulate_user_input` → message_added with role=user → message_completed |

**Validation checks (after all phases):**
- Event-level: correct event types, completions for each phase, streaming order
- Store state: sessions created with ZedThreadID, interactions with non-empty ResponseMessage
- Multi-thread: Thread A and Thread B map to different sessions in store
- Streaming: message_added events arrive before message_completed for each request
- Accumulation: completed interactions have ResponseMessage content (Bug 1 fix)
- Context mappings: correct thread→session mappings in `srv.ContextMappings()`

**How to run:**
```bash
# Build test server (from Zed repo)
cd ~/pm/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server
CGO_ENABLED=0 go build -o helix-ws-test-server .

# Run in Docker (Dockerfile.runtime uses pre-built binaries)
cd ~/pm/zed/crates/external_websocket_sync/e2e-test
docker build -t zed-ws-e2e -f Dockerfile.runtime .
docker run --rm zed-ws-e2e
```

### 3. Zed-Side Mock Server Tests (Rust) — EXISTS

**File:** `~/pm/zed/crates/external_websocket_sync/src/mock_helix_server.rs`
**Tests:** 19 tests (13 mock server integration + 6 serialization)

`MockHelixServer` is a standalone Rust mock of Helix's WebSocket endpoint. It implements the readiness protocol (queue commands until `agent_ready`), accepts multiple concurrent sessions, and validates auth tokens. This mock does NOT run real Helix handlers — it's a protocol-level mock for testing the Zed client in isolation.

These tests verify the Zed client connects correctly and exchanges messages with a mock Helix:

| Test | What It Covers |
|------|---------------|
| `test_mock_server_starts_and_accepts_connection` | Basic WebSocket connect |
| `test_mock_server_records_received_events` | Event recording (agent_ready) |
| `test_mock_server_readiness_protocol_queues_commands` | Queuing before agent_ready, flush after |
| `test_mock_server_sends_commands_immediately_when_ready` | Post-ready immediate send |
| `test_mock_server_auth_token_validation_accepts_valid` | Bearer token auth |
| `test_mock_server_auth_token_validation_rejects_invalid` | Invalid token rejection |
| `test_mock_server_multiple_sessions` | Concurrent sessions tracked independently |
| `test_mock_server_send_open_thread` | open_thread command delivery |
| `test_mock_server_get_events_of_type` | Event filtering |
| `test_mock_server_clear_events` | Event clearing |
| `test_mock_server_wait_for_event_count` | Waiting for specific event count |
| `test_mock_server_full_protocol_flow` | Complete lifecycle: connect → queue → agent_ready → flush → thread_created → 3x message_added → message_completed |
| `test_mock_server_send_command_immediate_bypasses_queue` | Bypass readiness queue |
| Serialization tests (6) | SyncEvent/OutgoingMessage/IncomingChatMessage/ReceivedSyncMessage serialization round-trips |

### 3. Zed-Side Protocol Tests (Rust, `#[ignore]`) — EXIST, NOT RUNNING

**File:** `~/pm/zed/crates/external_websocket_sync/src/protocol_test.rs`
**Wired in:** `mod protocol_test;` in `external_websocket_sync.rs:802`
**Tests:** 2 tests (both `#[ignore]`, require `HELIX_SESSION_ID` env var)

These connect Zed's real `WebSocketSync::start()` client to a mock external system (a raw TCP WebSocket server spun up in-test — NOT MockHelixServer, NOT real Helix). A spawned task handles `ThreadCreationRequest` callbacks, simulating what Zed's agent_panel/thread_service would do.

| Test | What It Covers |
|------|---------------|
| `test_end_to_end_protocol_flow` | chat_message → thread_created → 3x message_added (streaming) → message_completed |
| `test_follow_up_message_flow` | Thread reuse with existing acp_thread_id, no second thread_created |

**Problem:** These are `#[ignore]` and use `init_thread_creation_callback()` which replaces the global callback — can't run in parallel with other tests. They also don't use `MockHelixServer` (they predate it).

### 4. Dead Test Files (Not Compiled) — DELETE

**File:** `~/pm/zed/crates/external_websocket_sync/src/real_websocket_tests.rs`
**File:** `~/pm/zed/crates/external_websocket_sync/src/test_integration.rs`

These files are **NOT** `mod`-declared anywhere — they're orphaned and never compiled. Confirmed by:
- No `mod real_websocket_tests;` or `mod test_integration;` in any `.rs` file
- `cargo test -p external_websocket_sync` shows 39 tests total (37 pass + 2 ignored), none from these files

They reference removed APIs from an earlier protocol revision:
- `WebSocketSync::new()` (now `WebSocketSync::start()`)
- `helix_url` / `session_id` config fields (now `url` / removed)
- `ChatResponse` / `ChatResponseChunk` / `ChatResponseDone` / `ChatResponseError` event types (removed from `SyncEvent` enum)
- `event_type_string()` / `event_to_data()` methods (removed)
- Hardcoded ports 9001-9003 (fragile, no `MockHelixServer`)

**Action:** Delete both files. They provide no value — they don't compile and reference an obsolete protocol.

### 5. Real Zed E2E Coverage Summary

The Go test server (layer 2) exercises both real Helix handlers AND real Zed binary. Current coverage across all layers:

---

## Coverage Gap Analysis

### What's Covered

| Code Path | Go Unit | Go+Zed E2E | Zed Mock Server |
|-----------|---------|------------|-----------------|
| agent_ready + readiness protocol | Yes | Yes (phase 1) | Yes |
| chat_message → thread_created (new thread) | Yes | Yes (phase 1, 3) | Yes |
| chat_message → follow-up (existing thread) | Partial | Yes (phase 2, 4) | Yes (protocol_test.rs) |
| message_added streaming (same msg_id) | Yes | Yes (validated) | Yes |
| message_added multi-entry (new msg_id) | Yes | Implicit | No |
| message_completed → state=Complete | Yes | Yes (all phases) | Yes |
| simulate_user_input (Zed→Helix) | No | Yes (phase 5) | No |
| user_created_thread | Yes | No | Serialization only |
| thread_title_changed | Yes | No | Serialization only |
| thread_load_error | No | No | Serialization only |
| contextMapping miss → DB fallback | Yes | No | N/A |
| processPromptQueue | Yes | No | No |
| processAnyPendingPrompt | Yes | No | No |
| Design review comment linking | Yes | No | No |
| Design review comment finalization | Yes | No | No |
| SpectaskLink (copy SpecTaskID) | Yes | No | No |
| Reconnection (ses_ prefix, restore state) | No | No | No |
| Non-visible thread follow-up | No | Yes (phase 4) | No |
| Multi-thread (different sessions in store) | No | Yes (validated) | Yes (mock only) |
| Store state validation (sessions, interactions) | N/A | Yes (validated) | N/A |
| Accumulation validation (ResponseMessage content) | Yes | Yes (validated) | No |
| Concurrent multiple agents | No | No | Yes (mock only) |
| Auth token validation | No | No | Yes |
| open_thread command | No | No | Yes |
| query_ui_state command | No | No | No |

### Critical Gaps — Not Covered by Any Test Layer

These code paths are tested by neither Go unit tests nor the E2E test server:

1. **Reconnection / Session Resume** — When a Zed session restarts and reconnects with the same `session_id`, does the state restore correctly? Does `agent_ready` include the correct `thread_id`? The Go handler has reconnection logic at `websocket_external_agent_sync.go:259-395` but nothing exercises it.

2. **user_created_thread (real Zed)** — The Go unit test covers the handler, but no E2E test triggers a real user creating a thread in Zed UI. The E2E only tests server-initiated `chat_message` and `simulate_user_input`.

3. **thread_title_changed (real Zed)** — Go unit test covers the handler, but the E2E doesn't verify Zed actually sends this event when the agent generates a title.

4. **thread_load_error** — No test at any layer. When `open_thread` targets a corrupted/conflicting thread, Zed should send this.

5. **open_thread command (real Zed)** — MockHelixServer tests delivery of the command, but no E2E test verifies Zed actually opens the thread and sends the correct response events.

6. **Prompt queue (real Zed)** — Go unit tests cover `processPromptQueue` and `processAnyPendingPrompt`, but no E2E test sends a second prompt while the agent is still processing the first to verify queuing works end-to-end.

7. **query_ui_state** — Not tested anywhere. New command that should return `ui_state_response`.

8. **Design review comments (real Zed)** — Go unit tests cover `linkAgentResponseToComment` and `finalizeCommentResponse`, but these are spectask-specific paths that would need a spectask E2E test environment.

---

## Plan for Expanding E2E Coverage

The E2E infrastructure already exists (`helix-ws-test-server` + `run_e2e.sh` + Docker). Expansion means adding more phases to `main.go` and more validation to `validate()`.

### New Phases to Add to helix-ws-test-server/main.go

| # | Phase | What to Add |
|---|-------|-------------|
| 6 | **open_thread** | Send `open_thread` with existing acp_thread_id → verify Zed opens thread and sends agent_ready/events |
| 7 | **Reconnection** | Kill Zed WS connection, wait for reconnect → verify agent_ready with thread_id, verify queued commands flush |
| 8 | **Prompt queue** | Send chat_message while agent is busy processing phase 6 → verify it completes after phase 6 |
| 9 | **query_ui_state** | Send query_ui_state → verify ui_state_response with correct active_view/thread_id |
| 10 | **thread_title_changed** | After a response completes, verify thread_title_changed event was received |

### Cleanup

1. Delete `real_websocket_tests.rs` and `test_integration.rs` — orphaned, never compiled, reference removed APIs
2. Consider removing `protocol_test.rs` — the 2 `#[ignore]` tests are superseded by the Go E2E test server
3. Rebuild the Go test server binary after any changes to Helix handler code

---

## Production Bug Found

### processNextCommentInQueue nil pointer dereference

**File:** `api/pkg/server/spec_task_design_review_handlers.go:646-655`

```go
comment, err := s.Store.GetNextQueuedCommentForSession(ctx, sessionID)
if err != nil {
    log.Debug().Str("session_id", sessionID).Msg("No queued comments to process")
    return
}
// BUG: If GetNextQueuedCommentForSession returns (nil, nil),
// the next line panics on comment.ID
log.Info().Str("comment_id", comment.ID).Msg("Processing next comment from database queue")
```

If `GetNextQueuedCommentForSession` returns `(nil, nil)` — which is valid GORM behavior when no record is found — the code panics with a nil pointer dereference on line 655 accessing `comment.ID`.

**Workaround in tests:** MockStore returns `(nil, store.ErrNotFound)` instead of `(nil, nil)`.
**Proper fix needed:** Add nil check for `comment` after the error check, or ensure the store method always returns `ErrNotFound` when no record exists.

---

## Files Reference

### Helix (Go)
- `api/pkg/server/websocket_external_agent_sync.go` — Main handler file (~2700 lines)
- `api/pkg/server/websocket_external_agent_sync_test.go` — Unit tests (46 tests, all passing)
- `api/pkg/server/test_helpers.go` — `NewTestServer`, `QueueCommand`, etc.
- `api/pkg/server/spec_task_design_review_handlers.go` — Comment linking/finalization
- `api/pkg/store/memorystore/memorystore.go` — In-memory store for integration tests
- `api/pkg/pubsub/noop.go` — NoopPubSub for tests

### Zed (Rust)
- `~/pm/zed/crates/external_websocket_sync/src/websocket_sync.rs` — WebSocket client
- `~/pm/zed/crates/external_websocket_sync/src/thread_service.rs` — Thread management
- `~/pm/zed/crates/external_websocket_sync/src/types.rs` — Protocol types (SyncEvent enum)
- `~/pm/zed/crates/external_websocket_sync/src/mock_helix_server.rs` — Mock server + 19 tests
- `~/pm/zed/crates/external_websocket_sync/src/protocol_test.rs` — 2 `#[ignore]` E2E tests
- `~/pm/zed/crates/external_websocket_sync/src/real_websocket_tests.rs` — STALE (older API)
- `~/pm/zed/crates/external_websocket_sync/src/test_integration.rs` — STALE (older API)
