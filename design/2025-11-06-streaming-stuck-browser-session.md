# Streaming Stuck Browser Session - Root Cause Analysis

## Date
2025-11-06

## Reproduction Steps

1. **Initial State**: No agents, no streaming sessions, no lobbies
2. **Fork Sample Project** (16:13:42)
   - Created Modern Todo App from sample
   - Project ID: `26902767-9340-4d79-8e26-7bcb085c6956`

3. **Start Exploratory Session** (16:14:01)
   - Session ID: `ses_01k9cz4hxwbj8vjhjz40vc1kns`
   - Initial lobby: `4648b231-6a09-45d2-a3cc-90e82e029f5b`

4. **Test Startup Script Multiple Times** (16:18-16:26)
   - 16:18:33 - Restart #1 (new lobby: `5d423774`)
   - 16:19:05 - Restart #2 (new lobby: `163296bd`)
   - 16:19:50 - Restart #3 (new lobby: `6c9a8aa3`)
   - 16:20:43 - Restart #4
   - Each restart creates a **new Wolf lobby** but keeps **same Helix session ID**

5. **Navigate to Project "View Session"**
   - Connected to exploratory session
   - Disconnected and reconnected multiple times

6. **Start Planning on Backlog Item**
   - Clicked through to planning session
   - Full-screened the session

7. **Try Live Stream** (~17:04-17:11)
   - Live stream appeared to connect
   - Immediately showed: **"Stream terminated. Error 0"**
   - Reconnect button appeared
   - Reconnect attempts failed repeatedly
   - Browser refresh fixed the issue

## Log Analysis

### Key Findings

**1. Browser Client UUID Persistence**
From Moonlight Web logs, the browser has a consistent client UUID:
```
client_unique_id: ef28c5dc-cc65-441e-a276-82853986b281
```

This persists across:
- Different Helix session IDs
- Multiple lobby restarts  
- Browser tab navigation

**2. Moonlight WebSocket Churn (17:04-17:11)**
API logs show constant WebSocket reconnect loop:
```
17:04:21 - WebSocket connect → close 1005 (3 seconds)
17:04:24 - WebSocket connect → close 1005 (4 seconds)
17:04:28 - WebSocket connect → close 1005 (5 seconds)
17:05:03 - WebSocket connect → close 1005 (10 seconds)
... pattern continues every 10-15 seconds
```

Close code 1005 = "No status received" - server closed without proper close frame

**3. Missing Moonlight Web Activity (17:04-17:11)**
Moonlight Web logs **silent** during the problem period. No session creation attempts logged.

This suggests:
- Frontend WebSocket connects to API proxy
- API proxy tries to connect to Moonlight Web backend
- Moonlight Web immediately rejects (or doesn't see the connection)
- API closes the frontend connection with 1005

### Hypothesis: Stale Moonlight Web State

**Root Cause Theory:**

When testing the startup script multiple times:
1. **Each restart creates a new Wolf lobby** (new lobby ID)
2. **Moonlight Web creates streaming session** keyed by `agent-{helix-session-id}-{client-uuid}`
3. **Old streaming session NOT cleaned up** when lobby changes
4. **Browser tries to reconnect** with same session+client combo
5. **Moonlight Web has stale session state** pointing to dead lobby
6. **Connection fails immediately** with no error detail (close 1005)

**Evidence:**
- Same client UUID across all attempts: `ef28c5dc-cc65-441e-a276-82853986b281`
- Session ID unchanged: `ses_01k9cz4hxwbj8vjhjz40vc1kns`
- Lobby IDs keep changing: `4648b231` → `5d423774` → `163296bd` → `6c9a8aa3`
- Moonlight Web last activity: 16:16:38 (48 minutes before problem!)
- Frontend retry pattern: exponential backoff (3s, 4s, 5s, 10s, 10s...)

## Code Paths to Investigate

1. **Moonlight Web Session Cleanup**
   - When is streaming session cleaned up?
   - Does it clean up when lobby ID changes?
   - File: `moonlight-web` container logs show "Cleaning up session"

2. **Moonlight Proxy in API**
   - `pkg/server/moonlight_proxy.go:182` - WebSocket upgrade
   - `pkg/server/moonlight_proxy.go:324` - Connection close
   - Does it pass lobby ID or just session ID?

3. **Browser Client UUID Generation**
   - Where is `ef28c5dc-cc65-441e-a276-82853986b281` generated?
   - Is it localStorage? sessionStorage? Generated per tab?
   - Format: `helix-agent-{session-id}-{client-uuid}`

4. **Wolf Lobby ID Changes**
   - Each restart creates new lobby
   - Does Moonlight Web get notified of lobby change?
   - Does old streaming session linger?

## Questions to Answer

1. How is the Moonlight streaming session key generated?
   - Looks like: `agent-{helix-session-id}-{client-uuid}`
   - Does this key persist when lobby changes?

2. When does Moonlight Web clean up streaming sessions?
   - On lobby stop?
   - On client disconnect?
   - On timeout?

3. Why does browser refresh fix it?
   - New client UUID?
   - Clears frontend state?
   - Forces new Moonlight session creation?

4. What's the WebSocket connection flow?
   ```
   Browser → API (/moonlight/host/stream) → Moonlight Web (ws://moonlight-web:8080/api/host/stream)
   ```
   What session identifiers are passed through this chain?

## Next Steps

1. Trace through `moonlight_proxy.go` to see what's sent to Moonlight Web
2. Check if lobby ID is included in WebSocket connection
3. Verify Moonlight Web cleans up old sessions when lobby changes
4. Add lobby ID to Moonlight streaming session key (if missing)
5. Test: restart session multiple times, verify streaming works without browser refresh

## Temporary Workaround

User discovered: **Refresh browser** to fix stuck streaming.

This works because it:
- Clears frontend WebSocket state
- May generate new client UUID (need to verify)
- Forces Moonlight Web to create fresh streaming session

## Timeline Summary

```
16:13:42 - Fork sample project
16:14:01 - Start exploratory session #1 (lobby: 4648b231, session: ses_01k9cz4h)
16:18:33 - Restart (test startup #1) → new lobby: 5d423774
16:19:05 - Restart (test startup #2) → new lobby: 163296bd
16:19:50 - Restart (test startup #3) → new lobby: 6c9a8aa3
16:20:43 - Restart (test startup #4) → (lobby unknown from logs)
17:04:21 - Try to connect to stream → FAIL (WebSocket close 1005)
17:04-17:11 - Retry loop every 10s → all FAIL
[Browser refresh]
17:18:18 - Stream connects successfully
```

**48 minute gap** between last Moonlight activity (16:16:38) and first failed connection (17:04:21).

During this gap, user likely:
- Navigated to settings page
- Viewed session
- Started planning on backlog item
- Tried to fullscreen/stream


## Additional Symptom: Stream Busy Loop

After "Stream terminated. Error 0":
1. User clicked back to Screenshot view
2. Clicked back to Live Stream
3. Entered **"Stream busy" infinite retry loop**

### Stream Busy Behavior

From `MoonlightStreamViewer.tsx:284`:
```typescript
if (errorMsg.includes('AlreadyStreaming') || errorMsg.includes('already streaming')) {
  // Progressive retry: 2s, 3s, 4s, 5s... (capped at 10s)
  // Shows: "Stream busy (attempt N) - retrying in X seconds..."
}
```

**Trigger**: Moonlight Web returns `AlreadyStreaming` error

**Frontend Response**:
- Shows "Stream busy" message with countdown
- Auto-retries with exponential backoff (2s → 3s → 4s → ... → 10s max)
- Loop continues until success or user clicks away

### Root Cause Confirmed

**The Problem:**

Moonlight Web session key: `agent-{helix-session-id}-{client-uuid}`

When you restart exploratory session (test startup script):
- ✅ **Same** Helix session ID: `ses_01k9cz4hxwbj8vjhjz40vc1kns`
- ❌ **New** Wolf lobby ID: `4648b231` → `5d423774` → `163296bd` → `6c9a8aa3`
- ✅ **Same** browser client UUID: `ef28c5dc-cc65-441e-a276-82853986b281`

**Result:**
1. Old streaming session key: `agent-ses_01k9cz4h-ef28c5dc` → points to **dead lobby 4648b231**
2. New lobby created: `6c9a8aa3` (no streaming session registered)
3. Browser tries to connect with same key
4. Moonlight Web says "AlreadyStreaming" (old session exists!)
5. But old session is broken (points to dead lobby)
6. **Infinite retry loop** because cleanup never happens

**Why Browser Refresh Fixes It:**

Option A: Generates new browser client UUID → new Moonlight session key → avoids stale session
Option B: Frontend state reset allows re-initialization

Need to verify which by checking client UUID generation in frontend code.

## Critical Finding

**Moonlight Web session cleanup is NOT triggered when:**
- Wolf lobby changes (restart session)
- Old lobby stops
- New lobby starts with same Helix session ID

**Session IS cleaned up when:**
- Browser explicitly closes stream (seen in logs: "Cleaning up session")
- Session timeout (unknown duration)

## The Fix

**Option 1: Include lobby ID in Moonlight session key**
```
agent-{helix-session-id}-{lobby-id}-{client-uuid}
```
Each lobby restart gets unique Moonlight session → no conflicts

**Option 2: Clean up Moonlight sessions when lobby changes**
When restarting session:
1. Stop old Wolf lobby
2. **Tell Moonlight Web to cleanup old streaming session**
3. Start new lobby
4. New streaming session can be created

**Option 3: Frontend generates new client UUID on each connection**
Less ideal - loses some benefits of persistent client ID

## Recommended Solution

**Include lobby ID in session key** (Option 1):
- Simple, no API changes needed
- Moonlight Web naturally creates new session per lobby
- Old sessions auto-cleanup on timeout
- No race conditions

Implementation in `moonlight_proxy.go`: pass `wolf_lobby_id` to Moonlight Web

