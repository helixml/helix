# Immediate Lobby Attachment - Security Model and Architecture

**Date:** 2026-01-02
**Status:** In Progress

## Problem Statement

When Moonlight sessions join a Wolf lobby, the current flow involves:
1. Session starts with a test pattern producer
2. Session joins lobby via `JoinLobby` API
3. Wolf switches interpipesrc to lobby's interpipe producer
4. This interpipe switching causes format mismatch issues and video hangs

The `immediate_lobby_id` feature bypasses interpipe switching by attaching sessions directly to the lobby's interpipe at creation time.

## Challenge: Moonlight Protocol Limitation

The Moonlight protocol is fixed - we cannot add custom fields like `immediate_lobby_id` to the session creation request. Sessions are created when Moonlight clients (via moonlight-web-stream) connect to Wolf.

## Solution: Pre-Configuration Side-Channel

Since we can't modify the Moonlight protocol, we use a side-channel:

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Session Pre-Configuration Flow                                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. User requests access to External Agent session                      │
│     └── Helix API (RBAC check: does user have access?)                  │
│                                                                         │
│  2. Helix creates lobby (if needed) and pre-configures session          │
│     └── Wolf API: POST /api/v1/sessions/configure                       │
│         {                                                               │
│           "client_unique_id": "helix-agent-{sessionId}",                │
│           "immediate_lobby_id": "{lobbyId}"                             │
│         }                                                               │
│                                                                         │
│  3. Frontend connects to moonlight-web with known client_unique_id      │
│     └── WebSocket: { session_id: "agent-{id}", client_unique_id: "..." }│
│                                                                         │
│  4. Moonlight-web connects to Wolf with that client_unique_id           │
│     └── Moonlight protocol (no custom fields needed)                    │
│                                                                         │
│  5. Wolf looks up pending configuration by client_unique_id             │
│     └── Applies immediate_lobby_id to the new session                   │
│     └── Session attaches directly to lobby's interpipe                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## client_unique_id Pattern

The `client_unique_id` is deterministic and controlled by Helix:

- **External Agent sessions**: `helix-agent-{sessionId}`
- **Browser sessions with instance ID**: `helix-agent-{sessionId}-{lobbyId}-{instanceId}`

This pattern allows:
1. Helix to know the `client_unique_id` before the Moonlight connection occurs
2. Wolf to match incoming connections to pre-configured settings
3. Multiple browser tabs to connect with unique IDs (via `instanceId` suffix)

## Security Model

### Trust Boundaries

```
┌─────────────────────────────────────────────────────────────────────────┐
│ TRUSTED ZONE (internal to deployment)                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐     Unix Socket      ┌─────────────┐                   │
│  │ Helix API   │ ←─────────────────→ │    Wolf     │                   │
│  │ (RBAC)      │  /var/run/wolf.sock  │   Server    │                   │
│  └─────────────┘                      └─────────────┘                   │
│        ↑                                     ↑                          │
│        │ HTTP (authenticated)                │ Moonlight Protocol       │
│        │                                     │                          │
├────────┼─────────────────────────────────────┼──────────────────────────┤
│ UNTRUSTED ZONE (external)                    │                          │
├────────┼─────────────────────────────────────┼──────────────────────────┤
│        ↓                                     ↓                          │
│  ┌─────────────┐                      ┌─────────────┐                   │
│  │  Frontend   │                      │ Moonlight   │                   │
│  │  (Browser)  │ ←───────────────────→│  Web/Client │                   │
│  └─────────────┘    WebSocket         └─────────────┘                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Security Controls

1. **Helix RBAC is the Authority**
   - Before pre-configuring a session, Helix verifies the user has access to the external agent session
   - User access is checked via `authorizeUserToExternalAgentSession()` or similar
   - Only after RBAC passes does Helix call `ConfigurePendingSession`

2. **Wolf Unix Socket is Local-Only**
   - The `/api/v1/sessions/configure` endpoint is only accessible via Unix socket
   - Only containers with the socket mounted can call it (Helix API container)
   - External Moonlight clients cannot call this endpoint

3. **Moonlight Protocol Cannot Specify Lobbies**
   - The Moonlight protocol has no field for `immediate_lobby_id`
   - Clients can only specify their `client_unique_id` (which they know)
   - This means a malicious client cannot forge access to arbitrary lobbies

4. **client_unique_id is Predictable but Not Forgeable**
   - An attacker could guess `helix-agent-{sessionId}` patterns
   - BUT they would need to:
     a. Know a valid session ID
     b. Connect BEFORE the legitimate user (race condition)
     c. Have a pre-configuration already set up for that ID (requires Unix socket access)
   - Since step (c) requires Helix API access, the attack is not feasible

5. **Pending Configurations Have No Lobby PIN**
   - Regular Moonlight clients joining lobbies need the lobby PIN
   - Pre-configured sessions bypass PIN check because Helix already authorized access
   - This is safe because only Helix can create pre-configurations

### Regular Moonlight Clients (Future Work)

For regular Moonlight clients (not via Helix):
- No pre-configuration exists for their `client_unique_id`
- They connect normally and see the Wolf UI
- They can join lobbies manually (with PIN if required)
- Helix RBAC does not apply (they're not going through Helix)

This is the expected behavior - regular Moonlight is a separate access path with its own authorization (lobby PINs).

## Implementation Details

### Wolf C++ Changes

1. **New API Endpoint**: `/api/v1/sessions/configure`
   - Stores mapping: `client_unique_id` → `immediate_lobby_id`
   - Stored in `pending_session_configs` map in AppState

2. **Session Creation Hook**
   - When Moonlight session is created, check `pending_session_configs`
   - If match found: apply `immediate_lobby_id` to session
   - Delete the pending config (one-time use)

3. **Timeout/Cleanup**
   - Pending configs should expire after ~60 seconds
   - Prevents memory leaks if connections never happen

### Helix Go Changes

1. **ConfigurePendingSession Method** (DONE)
   - Added to `wolf/client.go`
   - Added to `WolfClientInterface`

2. **External Agent Creation Flow**
   - After creating lobby, call `ConfigurePendingSession`
   - Pass `helix-agent-{sessionId}` as `client_unique_id`
   - Pass lobby ID as `immediate_lobby_id`

### Frontend Changes

1. **Remove Auto-Join Polling**
   - Remove `autoJoinExternalAgentLobby` API calls
   - Remove polling loop in `MoonlightStreamViewer.tsx`
   - Session is already in lobby when it starts (no joining needed)

## Migration Path

1. Deploy Wolf C++ changes (new endpoint, session creation hook)
2. Deploy Helix API changes (call pre-configure before streaming)
3. Remove auto-join logic from frontend/backend (cleanup)
4. Test with existing external agent flows

## Resource Optimization

When `immediate_lobby_id` is set, Wolf skips creating:
1. **Test pattern video producer** - No `videotestsrc` pipeline needed
2. **Test pattern audio producer** - No `audiotestsrc` pipeline needed
3. **Wolf UI app runner** - No separate container/process for Wolf UI

This saves significant resources:
- ~2 GStreamer pipelines per session (video + audio test patterns)
- ~1 GPU encoder context per session (test pattern encoding)
- Memory for test pattern buffers

From `moonlight.cpp:211`:
```cpp
// Skip if immediate_lobby_id is set - session will attach directly to lobby's interpipe
if (session->app->video_producer_source.has_value() && !session->immediate_lobby_id.has_value()) {
```

## Testing

- Unit tests for `ConfigurePendingSession` in Go
- Integration test: create session, verify it's in lobby immediately
- Security test: attempt to connect with forged `client_unique_id` (should get test pattern, not lobby)
- Resource test: verify no test pattern pipelines created for immediate lobby sessions
