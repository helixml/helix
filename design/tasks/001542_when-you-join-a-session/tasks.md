# Implementation Tasks

- [x] In `websocket_external_agent_sync.go`, extract a `buildFullStatePatchEvent(sessionID, owner, interactionID string, entries []wsprotocol.ResponseEntry) ([]byte, error)` helper that builds a full-state `interaction_patch` event (all entries with `patch_offset=0`, no previous entries)
- [~] In `websocket_server_user.go`, after the pub/sub subscription is established (and before entering the read loop), check `apiServer.streamingContexts[sessionID]` under `streamingContextsMu.RLock()`; if an active streaming context exists, call `buildFullStatePatchEvent` with `sctx.accumulator.Entries()` and write the result directly to `conn` (under `wsMu`) as the catch-up snapshot
- [ ] Add a unit test in `websocket_external_agent_sync_test.go` (or a new file) verifying that a client connecting mid-stream receives a full-state patch containing all accumulated entries
