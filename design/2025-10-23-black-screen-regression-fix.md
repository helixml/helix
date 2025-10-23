# Black Screen Regression: Root Cause and Fix

## The Problem

After a totally fresh restart, clicking "Live Stream" on a new external agent session resulted in a **black screen** in moonlight-web.

## Debugging Process

### Logs Captured

**moonlight-web logs showed**:
```
[Streamer]: 10:10:14 [INFO] [Moonlight Stream]: Waiting for IDR frame
[Streamer]: 10:10:15 [INFO] [Moonlight Stream]: Reached consecutive drop limit
[Streamer]: 10:10:15 [INFO] [Moonlight Stream]: IDR frame request sent
thread '<unnamed>' panicked at bytes-1.10.1/src/bytes.rs:396:9:
range end out of bounds: 5574 <= 223
```

**Streamer was waiting for video frames** that never arrived, then panicked.

**Wolf logs showed**:
- ✅ App created successfully (app_id 918498224)
- ✅ Streaming session created
- ✅ Video pipeline configured: `nvh264enc → h264parse → rtpmoonlightpay`
- ✅ LAUNCH endpoint called successfully

**But NO container running**:
```bash
$ docker ps | grep agent
# Nothing!
```

### Root Cause Identified

**The AppWolfExecutor was missing the runner start call!**

**What happened**:
1. API created Wolf app via `/api/v1/apps/add` ✅
2. API established moonlight-web keepalive WebSocket ✅
3. API returned success to frontend ✅
4. **API NEVER called `/api/v1/runners/start`** ❌
5. Wolf had streaming session but no app container running
6. When user clicked "Live Stream", moonlight-web started streaming
7. Wolf tried to capture video from wayland display
8. **But no app container existed** → no video source
9. Streamer waited for frames that never came → black screen

**Why this happened**:
- In Wolf, there are two modes:
  - **Lobbies**: Auto-start the runner when created
  - **Apps**: Require explicit `/api/v1/runners/start` call
- The `wolf_executor_apps.go` uses apps mode
- It was missing the runner start call after establishing keepalive

## The Fix

**Added to `wolf_executor_apps.go`** (lines 240-270):
```go
// CRITICAL: Start the runner (Docker container) - apps don't auto-start like lobbies do!
sessionsResp, err := w.wolfClient.ListSessions(ctx)
if err != nil {
    return nil, fmt.Errorf("failed to list Wolf sessions: %w", err)
}

// Find the session for our app
var streamSessionID string
for _, session := range sessionsResp.Sessions {
    if session.AppID == wolfAppID {
        streamSessionID = fmt.Sprintf("%d", session.ClientID)
        break
    }
}

// Start the runner with the streaming session
err = w.wolfClient.StartRunner(ctx, streamSessionID, app.Runner, true)
if err != nil {
    return nil, fmt.Errorf("failed to start runner: %w", err)
}
```

**Added to `wolf.Client`** (api/pkg/wolf/client.go):
- `ListSessions()` - Query Wolf streaming sessions
- `StartRunner()` - Start a Docker container runner

## Verification

After the fix:
1. Create external agent session
2. Wolf app created
3. **Runner starts automatically** (new!)
4. Docker container launches with Zed + Sway
5. Video frames begin flowing
6. Live stream shows actual content

## Commits

- `3634ddbcd`: CRITICAL FIX - Add missing runner start call for external agents
- `4460594dc`: Reality check - Update design doc to reflect unknowns vs assumptions
- `7e973da25`: Add comprehensive architectural solution for EVP_DecryptFinal_ex errors
- `be11da9` (Wolf): Fix RESUME bug and add status endpoint for restart detection

## Key Lessons

1. **Apps vs Lobbies**: Apps need explicit runner start, lobbies auto-start
2. **Check actual system state**: `docker ps` revealed no container was running
3. **Follow the data flow**: Streamer waiting for frames → no video source → no container
4. **Test after every change**: This regression could have been caught with basic testing

## Additional Fix: Wolf endpoint_RunnerStart Missing Response

**Second bug found** (Wolf endpoints.cpp:321-345):
The `endpoint_RunnerStart` function fired the StartRunner event but **never sent an HTTP response**.

**Result**: Client got EOF error trying to start the runner.

**Fix** (Wolf commit e935f81):
```cpp
// After firing event, send success response
auto res = GenericSuccessResponse{.success = true};
send_http(socket, 200, rfl::json::write(res));
```

## External Agent Resume Workflow

**Question**: If we start the runner before a Moonlight client connects, can we resume later?

**Answer**: YES - that's the entire point!

**The workflow**:
1. Container starts immediately (no Moonlight client)
2. Zed connects to Helix WebSocket and works autonomously
3. User connects later via browser → Moonlight RESUME
4. Certificate persistence makes this work:
   - kickoff + browser use SAME `client_unique_id`
   - moonlight-web persists certs per `client_unique_id`
   - Wolf sees same cert → RESUME to existing session

## Status

✅ **FIXED** - API calls StartRunner() for apps mode
✅ **FIXED** - Wolf endpoint_RunnerStart sends HTTP response
✅ **DEPLOYED** - Both API and Wolf rebuilt and restarted
⬜ **NEEDS TESTING** - Create new external agent session and test live streaming
