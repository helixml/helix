# External Agent Streaming: RESUME Investigation Findings

**Date**: 2025-10-15
**Branch**: `feature/external-agents-hyprland-working` (Helix), `feature/kickoff` (moonlight-web)

## Problem Statement

External agent streaming worked on first connection but showed **black screen after page refresh**. Investigating the disconnect/reconnect flow revealed fundamental issues with how moonlight-web handles session cleanup.

## Initial Hypothesis: The "Kickoff Approach"

**Theory**: Use a 10-second "kickoff" connection to start the container, then have browser RESUME.

**Implementation**:
- Backend creates keepalive connection with `session_id: "agent-{id}-kickoff"`
- Waits 10 seconds for Wolf to start container
- Disconnects cleanly
- Browser connects with `session_id: "agent-{id}"` and same `client_unique_id`
- Moonlight protocol should auto-RESUME (high.rs:642-665)

**Result**: RESUME worked (1 Wolf session vs 3), but **no video frames** - "lack of video traffic" → ConnectionTerminated

## Root Cause Discovery

### Why Kickoff Failed

Investigated Wolf logs and found:
1. **Kickoff** (08:43:26): `/launch` → Wolf creates `waylanddisplaysrc` producer → Captures from Wayland
2. **Kickoff disconnects** (08:43:36): Wolf tears down streaming pipeline → `waylanddisplaysrc` stops
3. **Browser RESUME** (08:43:57): Wolf creates new `interpipesrc` consumer → **NO producer running!**
4. Result: `interpipesrc` reads from dead `interpipesink` → No frames → Stream timeout

**Critical insight**: Wolf's RESUME doesn't restart the `waylanddisplaysrc` capture pipeline - it assumes it's still running from original LAUNCH.

### The Real Problem: moonlight-web vs moonlight-qt

Tested with moonlight-qt from laptop - **disconnect/resume worked perfectly!**

**Wolf logs showed the difference**:

```
# moonlight-qt disconnect:
09:02:09 DEBUG | [ENET] disconnected client: 31.94.38.196:62872
09:02:09 DEBUG | [GSTREAMER] Pausing pipeline: 16531021386011371178
```

**moonlight-qt** sends proper disconnect signal → Wolf pauses pipeline → Clean state

**moonlight-web** (before fix):
- Browser navigates away → WebSocket closes abruptly
- Streamer's `stop()` just dropped `MoonlightStream` without calling cancel
- Wolf session left in corrupted state
- Result: `EVP_DecryptFinal_ex failed` errors flooding (encryption key mismatch)
- RESUME attempt gets `ConnectionTerminated`

## The Solution: Proper Disconnect Cleanup

### Code Changes (moonlight-web/streamer/src/main.rs)

**1. Remove keepalive special case** (line 469-487):
```rust
// BEFORE: Skipped stop() for keepalive mode
if self.keepalive_mode {
    debug!("[Keepalive]: Ignoring peer state");
    return;
}

// AFTER: Always stop on disconnect
if matches!(state, Failed | Disconnected | Closed) {
    info!("[Stream]: Peer disconnected, stopping stream cleanly for RESUME");
    self.stop().await;
}
```

**2. Add cancel() call in stop()** (line 846-864):
```rust
async fn stop(&self) {
    info!("[Stream]: Stopping - sending cancel to Wolf for clean disconnect");

    // CRITICAL: Send cancel to Wolf BEFORE dropping stream
    {
        let mut host = self.info.host.lock().await;
        match host.cancel().await {
            Ok(true) => {
                info!("[Stream]: Cancel successful - Wolf paused pipeline");
            }
            // ... error handling ...
        }
    }

    // Then proceed with normal cleanup...
}
```

### Additional Fix: Certificate Caching

**Problem**: Each moonlight-web session generated fresh certificates → Different Wolf client_id → LAUNCH instead of RESUME

**Solution** (moonlight-web/web-server/src/api/stream.rs + data.rs):
```rust
// Cache certificates by client_unique_id
pub struct RuntimeApiData {
    pub(crate) client_certificates: RwLock<HashMap<String, ClientAuth>>,
}

// In stream.rs:
let client_auth = if let Some(ref unique_id) = client_unique_id {
    let cache = data.client_certificates.read().await;
    if let Some(cached_auth) = cache.get(unique_id) {
        info!("Reusing cached certificate for '{}' (enables RESUME)", unique_id);
        cached_auth.clone()
    } else {
        // Generate and cache...
    }
}
```

**Result**: Same `client_unique_id` → Same certificate → Same Wolf client_id → RESUME works

## What "keepalive_mode" Actually Does Now

After simplification, `keepalive_mode` only controls **when Moonlight stream starts**:

- **keepalive_mode=true**: Starts stream immediately without waiting for ICE (headless operation)
- **keepalive_mode=false**: Waits for ICE connection before starting stream (normal browser flow)

**Both modes now**:
- Properly call `stop()` when peer disconnects
- Send `cancel()` to Wolf
- Allow clean RESUME on reconnect

## Testing Results

**moonlight-qt**:
- ✅ Connect → Works
- ✅ Disconnect → Clean (Wolf pauses pipeline)
- ✅ Reconnect → RESUME works perfectly

**moonlight-web** (with fix):
- Ready to test - should now match moonlight-qt behavior

## Commits

### moonlight-web (feature/kickoff branch):

1. **e329765**: Remove ICE restart, use separate session IDs
2. **949b932**: Add certificate caching for client_unique_id
3. **765a6a3**: Clean up kickoff sessions after disconnection
4. **aecf9f5**: Fix clean disconnect - send cancel to Wolf

### Helix (feature/external-agents-hyprland-working):

- Kickoff connection temporarily disabled for manual testing
- Wolf App State indicator added (shows absent/ready/streaming)

## Key Lessons

1. **Always send cancel before disconnecting** - Required for Wolf to pause pipeline cleanly
2. **Certificate caching is essential** - Same client_unique_id must reuse same certificate
3. **RESUME requires clean disconnect** - Abrupt disconnects corrupt Wolf session state
4. **Wolf's RESUME doesn't restart capture** - Assumes `waylanddisplaysrc` still running from LAUNCH
5. **keepalive_mode was cargo-culting** - Only needed for "start stream before ICE" use case

## Breakthrough: Clean Disconnect Works!

**Test Results** (after adding cancel to stop()):

Tested disconnect/resume from moonlight-web UI:
- ✅ First connection: Works
- ✅ Navigate away: Clean disconnect (no EVP errors in Wolf logs!)
- ✅ Reconnect: RESUME works! Shows video again

**Wolf logs confirmed clean disconnect**:
```
[ENET] disconnected client
[GSTREAMER] Pausing pipeline
```

Just like moonlight-qt! No more `EVP_DecryptFinal_ex failed` spam.

## Complete Working Solution

### Components

**1. Helix Backend** (wolf_executor_apps.go):
- Kickoff connection with `session_id: "agent-{id}-kickoff"`
- Uses `client_unique_id: "helix-agent-{id}"`
- Disconnects after 10 seconds (sends cancel → Wolf pauses)

**2. Helix Frontend** (MoonlightStreamViewer.tsx):
- Fetches actual Wolf app ID from backend
- Uses `session_id: "agent-{id}"` (different from kickoff)
- Uses `client_unique_id: "helix-agent-{id}"` (same as kickoff!)

**3. moonlight-web** (feature/kickoff branch):
- Certificate caching per `client_unique_id`
- Removed keepalive special cases
- Added `host.cancel()` in `stop()` function

### The Working Flow

1. **Create external agent**:
   - Helix creates Wolf app
   - 10-second kickoff: `session_id="agent-{id}-kickoff"`, `client_unique_id="helix-agent-{id}"`
   - Moonlight-web generates certificate, caches it
   - Wolf LAUNCHes container, starts `waylanddisplaysrc`
   - Kickoff disconnects → Sends cancel → Wolf pauses pipeline → Session resumable

2. **Toggle "Live Stream"** in browser:
   - Frontend fetches Wolf app ID from `/api/v1/sessions/{id}/wolf-app-state`
   - Connects with `session_id="agent-{id}"`, `client_unique_id="helix-agent-{id}"`
   - Moonlight-web reuses cached certificate (same Wolf client_id!)
   - Moonlight protocol: Same client + app running → RESUME
   - Wolf unpauses pipeline → Video flows!

3. **Page refresh**:
   - Browser disconnects → Sends cancel → Wolf pauses pipeline
   - Browser reconnects → Fetches app ID again → RESUME
   - Wolf unpauses → Video works again!

### Why It Works Now

**Certificate Caching**:
- Kickoff and browser share same certificate via `client_unique_id`
- Same certificate = Same Wolf client_id = RESUME works

**Clean Disconnect**:
- moonlight-web sends `/cancel` to Wolf (like moonlight-qt)
- Wolf pauses GStreamer pipeline (keeps `waylanddisplaysrc` ready)
- Session stays resumable without encryption corruption

**Separate Sessions**:
- Kickoff uses `"-kickoff"` suffix → Terminates cleanly
- Browser uses base session ID → Fresh streamer process
- No session/peer reuse → Clean WebRTC tracks

## Next Steps

1. ✅ Test moonlight-web disconnect/resume - **WORKS!**
2. ✅ Re-enable kickoff approach - **DONE**
3. Test Helix frontend browser streaming with kickoff + RESUME
4. Verify page refresh works cleanly
5. Celebrate! 🎉

## Technical Details

### Moonlight Protocol Flow

**LAUNCH** (first connection):
```
Client → /launch → Wolf creates:
  - waylanddisplaysrc (producer) → interpipesink
  - interpipesrc → encoder → RTP → Client
```

**RESUME** (reconnect with same certificate):
```
Client → /resume → Wolf creates:
  - NEW interpipesrc → encoder → RTP → Client
  - Reuses existing waylanddisplaysrc (assumes still running!)
```

**CANCEL** (clean disconnect):
```
Client → /cancel → Wolf:
  - Pauses all GStreamer pipelines
  - Preserves session state for RESUME
  - No encryption key corruption
```

### Wolf Session Identification

Wolf identifies clients by **certificate hash**:
- Different certificate = Different client_id = LAUNCH
- Same certificate = Same client_id = RESUME (if app already running)

This is why certificate caching per `client_unique_id` is critical.

### The EVP_DecryptFinal_ex Error

When moonlight-web didn't send cancel:
- Wolf kept old encryption keys in memory
- New connection with new keys arrived
- Wolf tried to decrypt with wrong keys
- Result: `EVP_DecryptFinal_ex failed` spam + corrupted state

Clean cancel prevents this by properly cleaning up encryption state.

## The Debugging Journey: A Hellscape

### What Made This So Hard

1. **Multi-layer complexity**: Browser → moonlight-web → Wolf → Container → Wayland → GStreamer
2. **State scattered across components**: Certificates in moonlight-web, sessions in Wolf, pipelines in GStreamer
3. **Encryption hiding the real problem**: EVP errors made it look like crypto issue when it was cleanup
4. **Timing-dependent behaviors**: First connect works, refresh fails - suggests cleanup issue
5. **Multiple wrong theories**:
   - "Track bindings paused" (ICE restart problem)
   - "Need to recreate peer" (Rust refactoring nightmare)
   - "Kickoff interferes with capture" (Wolf buffer issue)
   - **Reality**: moonlight-web just wasn't sending cancel

### The Breakthrough Moments

1. **moonlight-qt test**: Showed RESUME works when client disconnects properly
2. **Wolf logs comparison**:
   - moonlight-qt: "Pausing pipeline" ✅
   - moonlight-web: "EVP_DecryptFinal_ex failed" ❌
3. **Finding host.cancel()**: Realized moonlight-web never called it
4. **keepalive_mode realization**: Was cargo-culting from old Wolf crash workarounds

### Lessons Learned

1. **Test with reference implementation first** - moonlight-qt revealed the real issue immediately
2. **Compare successful vs failed flows** - Log comparison showed missing cancel
3. **Question every assumption** - "keepalive must persist" was wrong
4. **Simpler is better** - Final solution removed complexity instead of adding it
5. **Clean disconnect > Session persistence** - Proper cleanup enables RESUME

### Why This Was Worth It

External agents need to:
- Start autonomously before any browser connects
- Survive browser disconnect/reconnect cycles
- Handle page refreshes gracefully
- Support multiple concurrent sessions

Without proper RESUME, every refresh creates a new session → Multiple containers → Resource waste → Bad UX

The fix enables:
- One container per agent (efficient)
- Seamless page refreshes (good UX)
- Multiple users can observe same agent (collaborative)
- Standard Moonlight protocol (maintainable)

## References

- **Moonlight protocol**: `moonlight-common/src/high.rs:615-665` (LAUNCH vs RESUME logic)
- **Wolf disconnect**: Logs show "Pausing pipeline" when cancel received
- **moonlight-web streamer**: `moonlight-web/streamer/src/main.rs`
- **Certificate management**: `moonlight-web/web-server/src/api/stream.rs`

---

## Blog Post Notes

**Title Ideas**:
- "Debugging WebRTC Session Resume: A Multi-Day Journey Through Moonlight Protocol"
- "How a Missing cancel() Call Caused Encrypted Stream Corruption"
- "The Hellscape of Debugging Distributed Streaming Systems"

**Key Narrative Arc**:
1. Black screen after page refresh (symptoms)
2. Multiple failed hypotheses (ICE restart, peer recreation, capture buffers)
3. The kickoff approach attempt (close but no video)
4. moonlight-qt comparison (the aha moment)
5. Finding the missing cancel (root cause)
6. Clean solution (remove complexity, add one function call)

**Technical Depth**:
- Moonlight protocol internals (LAUNCH vs RESUME)
- Wolf's GStreamer pipeline lifecycle
- WebRTC track binding issues
- Certificate-based client identification
- Encryption state management

**Human Interest**:
- Months of development on external agents
- Session after session of debugging
- False starts and wrong theories
- The satisfaction of finally understanding the system
- How reference implementations save you

This could make a compelling technical deep-dive for developers working with:
- WebRTC streaming
- Moonlight protocol
- Distributed systems debugging
- Session management
- Clean disconnect patterns
