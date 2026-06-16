# Implementation Tasks: Add Clear Session Capability for ACP and Internal Agent Backends

## Store layer (TDD)
- [ ] Write failing test for `ClearSessionInteractions(ctx, sessionID)` in `api/pkg/store/store_interactions_test.go` (seed N interactions, clear, assert zero remain, session row preserved, idempotent on empty)
- [ ] Add `ClearSessionInteractions(ctx, sessionID) error` to the `Store` interface in `api/pkg/store/store.go`
- [ ] Implement it in `api/pkg/store/store_interactions.go` as a single `Where("session_id = ?").Delete(&types.Interaction{})`
- [ ] Regenerate the `Store` mock if one exists

## Compositional API: SessionBackend + coordinator (TDD)
- [ ] Define `SessionBackend` interface with `Clear(ctx, sessionID) error`
- [ ] Write failing test for `ClearSession` coordinator: always calls `ClearSessionInteractions`, dispatches to correct backend by `CodeAgentRuntime`/`ExternalAgentConfig`
- [ ] Implement `backendFor(session)` dispatch (zed/ACP vs internal agent)
- [ ] Implement `ClearSession` coordinator (clear DB → delegate backend → TouchSession → return session)

## Internal agent backend (TDD)
- [ ] Write test asserting `Clear` empties a live `agent.Session` `MessageList` and no-ops when no live session exists
- [ ] Implement `internalAgentBackend.Clear` using `GetMessageHistory().Clear()` (`api/pkg/agent/llm_message.go:87`)

## ACP / Zed backend (TDD)
- [ ] Write test (mocked WS sender + store) asserting: in-flight turn cancelled, new `ZedThreadID` persisted, fresh-thread command sent, no-connection path returns success
- [ ] Implement `zedACPBackend.Clear`: cancel current turn (`sendCancelToExternalAgent`), allocate + persist new `ZedThreadID` via `UpdateSessionMetadata`, send `open_thread` (or new `clear_thread`) command
- [ ] Add `clear_thread` command type in `websocket_external_agent_sync.go` ONLY if Zed requires an explicit forget signal (follow `cancel_current_turn` ack pattern); otherwise reuse `open_thread`
- [ ] Handle `ErrNoExternalAgentWS` as success (thread id persisted for next connect)

## HTTP endpoint (TDD)
- [ ] Write handler test: `POST /api/v1/sessions/{id}/clear` → 200 success, 404 unknown, 403 unauthorized
- [ ] Implement `clearSessionHandler` in `api/pkg/server/session_handlers.go` (auth like `deleteSession`, call `controller.ClearSession`)
- [ ] Register route in `api/pkg/server/server.go` sessions block

## Verification
- [ ] Run `go test ./api/pkg/store/... ./api/pkg/agent/... ./api/pkg/server/... ./api/pkg/controller/...` and ensure green
- [ ] Manual check: clear an internal-agent session, confirm next message starts fresh
- [ ] Manual check: clear a Zed-backed session, confirm Zed starts a clean thread and no stale tokens repopulate history
