# Implementation Tasks: Add Clear Session Capability for ACP and Internal Agent Backends

## Store layer (TDD)
- [x] Write failing test for `ClearSessionInteractions(ctx, sessionID)` in `api/pkg/store/store_interactions_test.go` (seed N interactions, clear, assert zero remain, session row preserved, idempotent on empty)
- [x] Add `ClearSessionInteractions(ctx, sessionID) error` to the `Store` interface in `api/pkg/store/store.go`
- [x] Implement it in `api/pkg/store/store_interactions.go` as a single `Where("session_id = ?").Delete(&types.Interaction{})`
- [x] Regenerate the `Store` mock if one exists

## Compositional API: SessionBackend + coordinator (TDD)
- [x] Define `SessionBackend` interface with `Clear(ctx, sessionID) error`
- [x] Write failing test for `ClearSession` coordinator: always calls `ClearSessionInteractions`, dispatches to correct backend by `CodeAgentRuntime`/`ExternalAgentConfig`
- [x] Implement `backendFor(session)` dispatch (zed/ACP vs internal agent)
- [x] Implement `ClearSession` coordinator (clear DB → delegate backend → TouchSession → return session)

## Internal agent backend (TDD)
- [x] Write test asserting `Clear` empties a live `agent.Session` `MessageList` and no-ops when no live session exists
- [x] Implement `internalAgentBackend.Clear` using `GetMessageHistory().Clear()` (`api/pkg/agent/llm_message.go:87`)

## ACP / Zed backend (TDD)
> Refined during implementation: the server cannot mint Zed-valid thread IDs (Zed
> creates them and persists in threads.db; `open_thread` only re-opens an EXISTING
> thread or it errors with thread_load_error). The canonical "start fresh" signal is
> `acp_thread_id = nil` on the next `chat_message`, which Zed turns into a brand-new
> thread (same path forks use). So `Clear` resets `ZedThreadID` to "" — the next
> message naturally opens a clean Zed thread, discarding prior context. No new
> `clear_thread` protocol command is needed.
- [x] Write test (mocked WS transport + store) asserting: in-flight turn cancelled, `ZedThreadID` reset to "" and persisted via `UpdateSessionMetadata`, no-connection path returns success
- [x] Implement `zedACPBackend.Clear`: cancel current turn (`cancelCurrentTurnIfActive`), reset + persist empty `ZedThreadID` via `UpdateSessionMetadata` so next message opens a fresh thread
- [x] ~~Add `clear_thread` command type~~ — not needed; reuse the existing nil-thread-id new-thread path
- [x] Handle no-connection case as success — cancel is best-effort and no WS command is sent, so a disconnected agent never surfaces an error

## HTTP endpoint (TDD)
- [x] Write handler test: `POST /api/v1/sessions/{id}/clear` → 200 success, 404 unknown, 403 unauthorized
- [x] Implement `clearSessionHandler` (in `api/pkg/server/session_clear.go` alongside the coordinator/backends for cohesion; auth like `deleteSession`, calls `apiServer.ClearSession`)
- [x] Register route in `api/pkg/server/server.go` sessions block

## Verification
- [x] `go test -run TestSessionClearSuite ./pkg/server/` green; `go test ./pkg/agent/` green; `go build ./...` green; `go vet` clean (pre-existing unrelated warning only). Store tests not run locally (need Postgres, per repo convention) — raw delete exercised by the e2e below. `pkg/agent/tests` integration suite fails pre-existing (needs live LLM, unrelated).
- [x] E2E against inner Helix (real Postgres): internal-agent session seeded with 3 interactions → `POST /sessions/{id}/clear` 200, interactions 3→0, session row preserved; idempotent re-clear 200; unknown 404; other-owner 403.
- [~] Manual check: clear a Zed-backed session — covered by unit tests (`TestZedBackend_*`, `TestClearSession_ZedDispatch`). Full desktop-stack manual run not performed; mechanism is the established nil-thread-id new-thread path (see design.md Implementation Notes).
