# Fix: Duplicate Stream Clients Accumulate for Desktop Sessions

**Date:** 2026-03-19
**Reporter:** Doug Underwood
**Versions affected:** Helix 2.9.3, 2.9.5

## Problem

When opening a single desktop session, the viewer repeatedly creates new stream/WebSocket clients for the same session and same user every few seconds. `total_clients` climbs from 2 to 60+ instead of staying at 1. The desktop backend and video pipeline remain healthy, but the accumulating clients cause noise, instability, and unnecessary load.

## Root Cause

Three interacting bugs:

### 1. Server heartbeat doesn't clean up dead connections (`ws_stream.go`)
The `heartbeat()` goroutine sends WebSocket pings every 5s. When the ping write fails (dead connection), the goroutine returned silently without closing the WebSocket. The main handler loop (`ws.ReadMessage()`) remained blocked indefinitely — `Stop()` never ran, so `UnregisterClient()` and `Unsubscribe()` from the shared video source never executed. Dead clients stayed registered and receiving video frame broadcasts forever.

### 2. Frontend never passes client instance ID (`DesktopStreamViewer.tsx`)
Each `DesktopStreamViewer` component generates a UUID (`componentInstanceIdRef`) to identify itself, but it was:
- Regenerated on every `connect()` call (defeating deduplication)
- Never passed to `WebSocketStream` (always `undefined`)

Without a stable instance ID, the backend had no way to identify reconnections from the same viewer tab.

### 3. No client deduplication on registration (`session_registry.go`)
`RegisterClient()` always created a new client with a fresh ID. It never checked if a client with the same viewer instance already existed. Each reconnection created a new client without evicting the old one.

## Fix

### heartbeat cleanup (`ws_stream.go`)
When ping write fails, close `v.ws` to unblock `ReadMessage()`. This triggers the defer chain: `Stop()` → `Unsubscribe()` + `UnregisterClient()`.

Added defense-in-depth: pong handler with 30s read deadline. If the client is dead and pongs stop arriving, the read deadline expires and `ReadMessage()` returns an error.

### Client instance ID (`DesktopStreamViewer.tsx`)
- Stop regenerating `componentInstanceIdRef` on reconnect — keep the UUID stable for the component's lifetime
- Pass it to `WebSocketStream` as `clientUniqueId`, which sends it in the init message

### Client deduplication (`session_registry.go`)
- Added `ClientUniqueID` field to `ConnectedClient`
- `RegisterClient()` now accepts `clientUniqueID` parameter
- Before creating a new client, scans existing clients for matching `ClientUniqueID`
- If found, closes the old WebSocket (triggering full cleanup) and removes from registry
- Multi-tab viewing remains safe: different tabs have different UUIDs

## Files Changed

- `api/pkg/desktop/ws_stream.go` — heartbeat fix, pong handler, pass ClientUniqueID
- `api/pkg/desktop/session_registry.go` — ClientUniqueID field, deduplication in RegisterClient
- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` — stable UUID, pass to WebSocketStream
