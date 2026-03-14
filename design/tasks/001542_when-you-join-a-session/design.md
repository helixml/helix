# Design: Late-Joiner Catch-Up for Active Streaming Sessions

## Architecture Context

### How streaming works (PR #1898)

```
Zed → message_added → Go API → MessageAccumulator → interaction_patch (delta) → pub/sub → Frontend
```

The `interaction_patch` event carries `EntryPatch[]`, each patch being a string delta relative to the previous published snapshot (`sctx.previousEntries`). The frontend applies these deltas to a `ResponseEntry[]` array maintained in `patchEntriesRef`.

### The streamingContext

`HelixAPIServer.streamingContexts` (map keyed by `helix_session_id`) holds active streaming state:

```go
type streamingContext struct {
    session         *types.Session
    interaction     *types.Interaction     // partially stale (200ms DB throttle)
    accumulator     *wsprotocol.MessageAccumulator  // in-memory, most up-to-date
    previousEntries []wsprotocol.ResponseEntry      // last published snapshot
    ...
}
```

`accumulator.Entries()` returns the most current structured entries (more up-to-date than `interaction.ResponseEntries` which is only written every 200ms to DB).

### The bug

`websocket_server_user.go` subscribes to the pub/sub queue and immediately enters a read loop. No catch-up is performed. A late-joining client starts with an empty `ResponseEntry[]` baseline, so delta patches like `{index:0, patch_offset:500, patch:"...more tokens"}` cannot be applied correctly — there is no content at offset 500.

## Fix: Send a Full-State Snapshot on Connect

In `websocket_server_user.go`, after subscribing to pub/sub (but before the read loop), check for an active `streamingContext` for the session. If one exists, build and send a catch-up `interaction_patch` event directly to the WebSocket connection.

### Ordering (critical for correctness)

```
1. Subscribe to pub/sub          ← ensures no delta patches are missed
2. Check streamingContexts       ← read accumulated entries under mutex
3. Send catch-up snapshot        ← direct write to conn (not pub/sub)
4. Enter read loop               ← normal operation
```

Subscribing before the snapshot means: if a new patch is published between steps 2 and 3, the client receives both (snapshot + incremental patch). The frontend applies them in order: snapshot initializes the baseline, then the incremental patch adds the new tokens. This is safe because patches from `previousEntries=nil` set absolute content (`patch_offset=0`), so idempotency holds even if an entry is "patched" twice.

### Catch-up event format

Reuse `types.WebsocketEvent{Type: WebsocketEventInteractionPatch}` with a full-state payload. Compute by calling `publishEntryPatchesToFrontend`'s patch logic with `previousEntries=nil` — every entry will have `PatchOffset=0` and `Patch=full content`.

Rather than calling `publishEntryPatchesToFrontend` (which publishes to pub/sub), extract the patch-computation logic into a helper, or inline the event construction in `websocket_server_user.go` and write it directly to `conn`.

**Recommended: new unexported helper**

```go
// buildFullStatePatchEvent builds an interaction_patch event with the full
// content of all entries (previousEntries=nil → patch_offset=0 for each).
// Used for late-joiner catch-up.
func buildFullStatePatchEvent(
    sessionID, owner, interactionID string,
    entries []wsprotocol.ResponseEntry,
) ([]byte, error)
```

Call from `websocket_server_user.go`:

```go
// After subscribe, before read loop:
apiServer.streamingContextsMu.RLock()
sctx, streaming := apiServer.streamingContexts[sessionID]
apiServer.streamingContextsMu.RUnlock()

if streaming && sctx != nil {
    sctx.mu.Lock()
    currentEntries := sctx.accumulator.Entries()
    interactionID := sctx.interaction.ID
    owner := sctx.session.Owner
    sctx.mu.Unlock()

    if len(currentEntries) > 0 {
        payload, err := buildFullStatePatchEvent(sessionID, owner, interactionID, currentEntries)
        if err == nil {
            wsMu.Lock()
            conn.WriteMessage(websocket.TextMessage, payload)
            wsMu.Unlock()
        }
    }
}
```

### Frontend changes

No frontend changes required. The `interaction_patch` handler in `streaming.tsx` already applies patches with `patch_offset=0` correctly — it sets the entry content to the patch string when offset is 0. Late-joining clients will receive a snapshot that populates their `patchEntriesRef`, then subsequent deltas apply normally.

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/server/websocket_server_user.go` | Add catch-up snapshot after subscription |
| `api/pkg/server/websocket_external_agent_sync.go` | Extract `buildFullStatePatchEvent` helper (or inline) |

## What to Avoid

- **Do not replay the entire pub/sub history** — NATS doesn't support this and it would be complex.
- **Do not send `interaction_update`** — the `sctx.interaction` is up to 200ms stale (DB throttle), while `accumulator.Entries()` has live data.
- **Do not publish catch-up to pub/sub** — that would broadcast to all connected clients, not just the late joiner.

## Codebase Patterns Found

- `streamingContexts` is protected by `streamingContextsMu sync.RWMutex`; use `RLock` to read.
- Individual `streamingContext` fields are protected by `sctx.mu sync.Mutex`.
- `websocket_server_user.go` already uses a `wsMu sync.Mutex` for thread-safe WebSocket writes (ping goroutine + subscription handler can race).
- `publishEntryPatchesToFrontend` contains the patch-computation loop that can be extracted into `buildFullStatePatchEvent`.
- `computePatch(prevContent, newContent)` computes a UTF-16 offset + string patch. With `prevContent=""`, it returns `(0, newContent, utf16Len(newContent))` — a full-content patch.
