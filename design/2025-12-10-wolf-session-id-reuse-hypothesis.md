# Wolf Crashes on Helix Session ID Reuse - Investigation & Hypothesis

**Date:** 2025-12-10
**Status:** Investigation Complete, Root Cause Identified

## Executive Summary

Wolf deadlocks when the same Helix session ID reconnects to the system. The root cause is that the Moonlight protocol allows only **one active stream per paired client at a time**. When a session with the same `client_unique_id` attempts to connect while a previous session is still cleaning up (or wasn't properly cleaned up), Wolf's internal state becomes corrupted.

## Observation

From user testing:
> "Yeah, the wolf crashes only happen when we reuse the session ID, but not when we don't, like in the Exploratory session test script."

Key insight: The crashes correlate with session ID reuse patterns, not with general Wolf usage.

## Session ID Architecture

### Helix Session IDs vs Wolf Sessions

There are **two different session concepts**:

1. **Helix Session IDs** (e.g., `ses_xxxxxxxxxxxxxx`)
   - Persistent identifiers for Helix chat sessions
   - Created in `api/pkg/server/session_handlers.go`
   - Used for chat history, interactions, and external agent tracking

2. **Wolf Sessions** (Moonlight protocol internal)
   - Created when a Moonlight client calls `launch` or `resume` endpoints
   - Identified by a hash of the client certificate: `std::hash<std::string>{}(client_cert)`
   - Stored in `state->running_sessions` immutable vector

### How Helix Session IDs Flow Into Wolf

The Helix session ID becomes part of the `client_unique_id` used in Moonlight:

```
Client Unique ID Format: helix-agent-{session_id}-{instance_id}

Example: helix-agent-ses_abc123-f7d2a1b3-c4e5-4f6a-8d9e-0a1b2c3d4e5f
```

This is constructed in `MoonlightStreamViewer.tsx:332`:
```typescript
const uniqueClientId = `helix-agent-${sessionId}${lobbyIdPart}-${componentInstanceIdRef.current}`;
```

## Moonlight Protocol Constraint

### One Client, One Stream

The Moonlight protocol enforces that **each paired client can only stream to one app at a time**. From `wolf.cpp:64`:
```cpp
bool is_busy = stream_session.has_value();
```

When a client is "busy" (has an active stream), the `serverinfo` response reports this state, and the client must either:
1. `cancel` the current stream before starting a new one
2. `resume` an existing paused stream

### Session Lookup by Client Certificate

Wolf's session identification uses the **client certificate hash**, not the `client_unique_id`. From `state/sessions.hpp`:
```cpp
inline std::optional<events::StreamSession> get_session_by_client(
    const immer::vector<events::StreamSession> &sessions,
    const wolf::config::PairedClient &client) {
  auto client_id = get_client_id(client);  // Hash of client_cert
  return get_session_by_id(sessions, client_id);
}
```

This means if two Helix sessions use the **same paired client** (which happens with auto-pairing), Wolf sees them as the same client trying to stream twice.

## The Crash Scenario

### Sequence of Events Leading to Deadlock

1. **First session starts**: Helix session `ses_A` creates Wolf session
   - Client unique ID: `helix-agent-ses_A-instance1`
   - Wolf creates `StreamSession` with session_id = hash(client_cert)
   - GStreamer pipelines start (video producer, audio producer, consumers)

2. **Session ends/pauses**: User stops streaming
   - `StopStreamEvent` fired (or `PauseStreamEvent` if just pausing)
   - Session should be removed from `running_sessions`
   - GStreamer pipelines should be destroyed

3. **Same Helix session restarts**: Same `ses_A` tries to reconnect
   - SAME client certificate used (auto-pairing cached)
   - Wolf sees hash(client_cert) == existing session's ID
   - **But the old session state may not be fully cleaned up!**

4. **State Corruption**:
   - New session creation overlaps with old session cleanup
   - GStreamer pipelines from old session still hold resources
   - GLib type locks held by old pipelines block new pipeline creation
   - Control thread tries to operate on corrupted session state

### Evidence from Thread Dump

The deadlocked threads were:
```
TID 20201: GStreamer-Pipeline (waylanddisplaysrc name=wolf_wayland_source...)
  Last heartbeat: 56s ago
  Status: STUCK

TID 945: Control-Server ()
  Last heartbeat: 56s ago
  Status: STUCK
```

The GStreamer pipeline thread and Control-Server getting stuck together suggests:
- The GStreamer pipeline is waiting on a lock (possibly GLib type lock)
- The Control thread is waiting on the pipeline to respond

## Why Fresh Session IDs Work

When a **new** Helix session ID is used:
- A new `componentInstanceIdRef.current` UUID is generated
- The `client_unique_id` is completely different
- Even if the same client certificate is used, the Wolf session is treated as a fresh connection
- No overlap with any in-progress cleanup from old sessions

## Root Cause Analysis

The fundamental issue is **race condition between session cleanup and session recreation**:

```
Time →

Old Session:  [active]--------------[StopStreamEvent]--[cleanup in progress...]
New Session:                                       ↑
                                                   [attempt to create new session]
                                                         ↓
                                               CONFLICT: Same client_id, old resources not freed
```

### Why This Doesn't Trigger in Exploratory Sessions

Exploratory sessions use:
1. Fresh Helix session IDs each time
2. Fresh lobby IDs each time
3. Fresh `componentInstanceIdRef.current` UUIDs each time

All of these combine to create completely unique `client_unique_id` values, avoiding the overlap.

## Affected Code Paths

### Session Creation (launch/resume)
- `wolf/src/moonlight-server/rest/endpoints.hpp:418-440` - `launch()` endpoint
- `wolf/src/moonlight-server/rest/endpoints.hpp:442-474` - `resume()` endpoint
- `wolf/src/moonlight-server/state/sessions.hpp:70-124` - `create_stream_session()`

### Session Cleanup
- `wolf/src/moonlight-server/sessions/moonlight.cpp:54-60` - `StopStreamEvent` handler
- `wolf/src/moonlight-server/control/control.cpp:146-158` - Control thread stop handler
- `wolf/src/moonlight-server/streaming/streaming.cpp:140-145` - Video pipeline stop handler

### Control Thread Session Lookup
- `wolf/src/moonlight-server/control/control.cpp:98-124` - `get_current_session()`
- Uses `enet_secret_payload` matching, not client_unique_id

## Potential Fixes

### Option 1: Force Session Cleanup Before Recreation
Before allowing a new session for the same client, ensure the old session is fully stopped:
```cpp
// In launch/resume endpoints
auto existing = state::get_session_by_client(state->running_sessions->load(), current_client);
if (existing) {
    // Fire StopStreamEvent and WAIT for cleanup
    state->event_bus->fire_event(immer::box<events::StopStreamEvent>(...));
    // Block until session is removed from running_sessions
    wait_for_session_cleanup(existing->session_id);
}
```

### Option 2: Include Lobby ID in Session Identification
Make session_id include the lobby ID, not just client cert hash:
```cpp
.session_id = get_client_id(current_client) ^ std::hash<std::string>{}(lobby_id)
```

This would make each lobby+client combination unique.

### Option 3: Helix-Side: Always Generate New Instance IDs
In Helix's frontend, ensure the `componentInstanceIdRef` is regenerated on each reconnect attempt to the same session:
```typescript
// Reset instance ID when session changes or on explicit reconnect
if (isRestart || sessionId !== previousSessionId) {
    componentInstanceIdRef.current = generateUUID();
}
```

### Option 4: Mutex Protection in Wolf
Add mutex protection around session creation/destruction to prevent race conditions:
```cpp
std::mutex session_lifecycle_mutex;

// In launch():
std::lock_guard<std::mutex> lock(session_lifecycle_mutex);
// ... create session ...

// In StopStreamEvent handler:
std::lock_guard<std::mutex> lock(session_lifecycle_mutex);
// ... cleanup session ...
```

## Implemented Fix

**Option 3 (Always Generate New Instance IDs)** was implemented as the simplest solution:

### Change 1: Fresh UUID per Connection (MoonlightStreamViewer.tsx)

```typescript
// In connect() callback - regenerate UUID at the START of every connection attempt
const connect = useCallback(async () => {
  // Generate fresh UUID for EVERY connection attempt
  // This prevents Wolf session ID conflicts when reconnecting to the same Helix session
  // (Wolf requires unique client_unique_id per connection to avoid stale state corruption)
  componentInstanceIdRef.current = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
  // ... rest of connection logic
```

This ensures every connection attempt (including reconnects) gets a unique `client_unique_id`, avoiding conflicts with stale sessions.

### Change 2: Certificate Cache Cleanup (moonlight-web-stream)

With fresh UUIDs per connection, the certificate cache in moonlight-web-stream could grow unbounded. Added:

1. **`CachedClientAuth` struct** with `last_used` timestamp
2. **Background cleanup task** that runs every 5 minutes
3. **1-hour expiry** for cached certificates

```rust
// In data.rs
pub struct CachedClientAuth {
    pub auth: ClientAuth,
    pub last_used: Instant,
}

// Cleanup task purges certificates idle > 1 hour
async fn certificate_cache_cleanup(data: Data<RuntimeApiData>) {
    loop {
        cleanup_interval.tick().await;
        cache.retain(|_, cached| !cached.is_expired(CERT_CACHE_MAX_IDLE));
    }
}
```

**Important:** The 1-hour timeout is safe because:
- Certificates are only needed at **connection time** for pairing
- Active sessions don't use the cached certificate (already paired)
- If a user disconnects for > 1 hour and reconnects, a new certificate is auto-generated
- The `touch()` method updates `last_used` when a certificate is reused

## Testing Recommendations

1. **Reproduce with instrumentation**: Add logging to session creation/destruction to capture timing
2. **Stress test session reuse**: Rapidly start/stop the same Helix session multiple times
3. **Test cleanup timing**: Measure how long GStreamer pipeline cleanup takes
4. **Race condition test**: Start new session immediately after stopping old one

## Appendix: Key Data Structures

### StreamSession (events.hpp:409-472)
```cpp
struct StreamSession {
  moonlight::DisplayMode display_mode;
  std::shared_ptr<EventBusType> event_bus;
  std::shared_ptr<App> app;
  std::string aes_key;
  std::string aes_iv;
  std::size_t session_id;  // Hash of client cert
  std::string ip;
  std::string client_unique_id;  // Moonlight uniqueid for secure session matching
  std::shared_ptr<immer::atom<virtual_display::wl_state_ptr>> wayland_display;
  std::shared_ptr<immer::atom<std::shared_ptr<audio::VSink>>> audio_sink;
  std::shared_ptr<std::optional<MouseTypes>> mouse;
  std::shared_ptr<std::optional<KeyboardTypes>> keyboard;
  // ... more fields
};
```

### Session Removal (state/sessions.hpp:126-133)
```cpp
inline immer::vector<events::StreamSession> remove_session(
    const immer::vector<events::StreamSession> &sessions,
    const events::StreamSession &session) {
  return sessions |
         ranges::views::filter([remove_hash = session.session_id](const events::StreamSession &cur_ses) {
           return cur_ses.session_id != remove_hash;
         }) |
         ranges::to<immer::vector<events::StreamSession>>();
}
```
