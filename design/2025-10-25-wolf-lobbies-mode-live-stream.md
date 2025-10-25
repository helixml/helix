# Wolf Lobbies Mode - Live Stream Integration

**Date:** 2025-10-25
**Status:** Implemented
**Context:** Transitioning from Wolf "apps" mode to "lobbies" mode for multi-user support

## Overview

Fixed the Live Stream functionality to work correctly with Wolf's lobbies mode. In lobbies mode, users connect to a Wolf UI browser interface where they can navigate to their lobby and enter a PIN, rather than connecting directly to a specific session's app.

## Background

### Wolf Modes

Wolf streaming server supports two operational modes:

1. **Apps Mode** (legacy):
   - Creates a separate Wolf app for each session
   - Direct connection: frontend → specific Wolf app
   - Single-user access per app
   - Required keepalive hack to prevent stale buffer crashes on client disconnect/reconnect

2. **Lobbies Mode** (new):
   - Creates lobbies that multiple users can join
   - Indirect connection: frontend → Wolf UI browser → lobby selection → PIN entry → lobby
   - Multi-user access with PIN protection
   - Lobbies persist naturally without keepalive hacks
   - More robust and scalable

### Configuration

Set in `docker-compose.dev.yaml`:
```yaml
environment:
  - WOLF_MODE=lobbies  # "apps" (default) or "lobbies" (multi-user with PIN support)
```

## Problem Statement

When attempting to use Live Stream in lobbies mode, users encountered "AppNotFound" errors:

```
ERR pkg/server/external_agent_handlers.go:644 > Failed to find external agent container
error="no external agent session found with Helix session ID: ses_..."
```

### Root Cause

The frontend was attempting to connect directly to a specific lobby's app ID, but lobbies mode works differently:

1. **Apps mode workflow:**
   - Frontend connects directly to Wolf app for session
   - `appId = wolfAppId` (e.g., 1972195426)

2. **Lobbies mode workflow (correct):**
   - Frontend connects to Wolf UI browser (appId=0)
   - User navigates through lobby list
   - User selects their lobby and enters PIN
   - User joins lobby and streams

The frontend was using the apps mode workflow in lobbies mode, trying to connect directly to a non-existent app.

## Solution

### 1. Frontend Streaming Components

#### MoonlightWebPlayer.tsx

Updated URL construction to use appId=0 when in lobbies mode:

```typescript
// Construct moonlight-web stream URL
// In lobbies mode (wolfLobbyId present): Connect to Wolf UI (appId=0) to browse lobbies
// In apps mode (no wolfLobbyId): Connect directly to specific app
// hostId=0 refers to the Wolf server configured in moonlight-web
const streamUrl = wolfLobbyId
  ? `/moonlight/stream.html?hostId=0&appId=0` // Lobbies mode: connect to Wolf UI browser
  : `/moonlight/stream.html?hostId=0&appId=1`; // Apps mode: connect to specific app
```

#### MoonlightStreamViewer.tsx

Updated connection logic to check for lobbies mode first:

```typescript
// Determine app ID based on mode
let actualAppId = appId;

if (wolfLobbyId) {
  // Lobbies mode: Connect to Wolf UI browser (app 0) where user will navigate to their lobby
  actualAppId = 0;
  console.log(`MoonlightStreamViewer: Using Wolf UI (app 0) for lobbies mode, lobby ${wolfLobbyId}`);
} else if (sessionId && !isPersonalDevEnvironment) {
  // Apps mode: Fetch the specific Wolf app ID for this session
  try {
    const wolfStateResponse = await apiClient.v1SessionsWolfAppStateDetail(sessionId);
    if (wolfStateResponse.data?.wolf_app_id) {
      actualAppId = parseInt(wolfStateResponse.data.wolf_app_id, 10);
      console.log(`MoonlightStreamViewer: Using Wolf app ID ${actualAppId} for session ${sessionId}`);
    }
  } catch (err) {
    console.warn('Failed to fetch Wolf app ID, using default:', err);
  }
}
```

**Key principle:** The presence of `wolfLobbyId` indicates lobbies mode, triggering the Wolf UI browser connection path.

### 2. Removed Keepalive UI

Since lobbies persist naturally without keepalive hacks, removed all keepalive UI elements from `SessionToolbar.tsx`:

- Removed keepaliveStatus state variable and interface
- Removed useEffect that polled `/api/v1/external-agents/{sessionID}/keepalive` every 10 seconds
- Removed `renderKeepaliveIndicator()` function displaying "Keepalive Starting/Active/Reconnecting/Failed" chips
- Removed the call to `renderKeepaliveIndicator()` in JSX

**Why this matters:** Keepalive was a workaround for apps mode instability. Lobbies mode doesn't need it, so the UI should reflect that.

## User Experience

### Lobbies Mode Flow

1. User clicks "Live Stream" button on an external agent session
2. Browser connects to Wolf UI (appId=0)
3. User sees lobby browser interface showing:
   - Available lobbies
   - Lobby names (e.g., "Agent wgvc")
   - Multi-user indicators
4. User clicks on their lobby
5. User enters lobby PIN (displayed in Helix UI)
6. User successfully joins lobby and begins streaming
7. **Multiple users can join the same lobby simultaneously**

### Apps Mode Flow (Backward Compatible)

1. User clicks "Live Stream" button
2. Browser connects directly to session's Wolf app
3. User immediately begins streaming
4. Single-user access only

## Technical Details

### Wolf API Endpoints

Check current mode and available resources:

```bash
# Check Wolf apps (apps mode)
docker compose -f docker-compose.dev.yaml exec api \
  curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps | jq '.'

# Check Wolf lobbies (lobbies mode)
docker compose -f docker-compose.dev.yaml exec api \
  curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies | jq '.'
```

### Mode Detection

Components detect lobbies mode through the presence of `wolfLobbyId` prop:
- If present: lobbies mode, connect to Wolf UI
- If absent: apps mode, connect to specific app

### Files Modified

1. `frontend/src/components/external-agent/MoonlightWebPlayer.tsx`
   - Updated stream URL construction for mode-aware connection

2. `frontend/src/components/external-agent/MoonlightStreamViewer.tsx`
   - Updated app ID determination logic with lobbies mode check

3. `frontend/src/components/session/SessionToolbar.tsx`
   - Removed all keepalive UI elements

### Commits

- `e218fcb27` - Remove keepalive UI completely from SessionToolbar - lobbies mode doesn't need keepalive
- `87530198c` - Fix Live Stream for lobbies mode - connect to Wolf UI (app 0) instead of specific lobby

## Testing

### Verify Lobbies Mode

```bash
# Check that WOLF_MODE is set to lobbies
grep "WOLF_MODE" docker-compose.dev.yaml

# Check that lobbies exist
docker compose -f docker-compose.dev.yaml exec api \
  curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies | jq '.lobbies[].name'
```

### Test Live Stream

1. Create an external agent session in Helix
2. Click "Live Stream" button
3. Verify connection to Wolf UI browser (check browser console for "Using Wolf UI (app 0)")
4. Verify lobby browser interface appears
5. Navigate to your lobby by name
6. Enter PIN from Helix UI
7. Verify successful connection and streaming

### Browser Cache Issues

If changes don't appear after deployment:
- **Hard refresh:** Ctrl+Shift+R (Windows/Linux) or Cmd+Shift+R (Mac)
- Frontend hot reload may not always clear JavaScript bundle cache

## Benefits of Lobbies Mode

1. **Multi-user support:** Multiple users can connect to the same session simultaneously
2. **PIN protection:** Each lobby requires a PIN, preventing unauthorized access
3. **Better stability:** Lobbies persist naturally without keepalive workarounds
4. **Cleaner architecture:** No need for per-client keepalive connections
5. **Scalability:** Wolf handles multiple clients per lobby more efficiently

## Migration Path

### For Existing Deployments

1. Update `WOLF_MODE=lobbies` in docker-compose.yaml
2. Restart API service: `docker compose restart api`
3. Frontend will automatically adapt based on `wolfLobbyId` presence
4. Old sessions in apps mode will continue working (backward compatible)
5. New sessions will use lobbies mode

### Rollback

If issues arise, revert to apps mode:
1. Set `WOLF_MODE=apps` in docker-compose.yaml
2. Restart API service
3. Frontend will automatically use apps mode (no wolfLobbyId)

## Future Improvements

1. **Wolf UI customization:** Brand the lobby browser with Helix styling
2. **Lobby metadata:** Display more context about each lobby (owner, active users, etc.)
3. **Auto-join:** Pre-fill PIN and auto-join lobby if user owns the session
4. **Lobby management UI:** Create/delete lobbies directly from Helix interface
5. **Connection indicators:** Show which users are currently connected to a lobby

## Lessons Learned

1. **Mode detection is critical:** Use `wolfLobbyId` presence as the source of truth for mode
2. **Wolf UI is appId=0:** This is the lobby browser interface in lobbies mode
3. **Remove obsolete workarounds:** Keepalive was a hack for apps mode instability
4. **Test both modes:** Ensure backward compatibility with apps mode during transition
5. **Browser caching matters:** Hard refresh required after JavaScript changes

## Related Documentation

- Wolf upstream: https://github.com/games-on-whales/wolf
- Wolf UI branch: wolf-ui (lobbies support)
- Moonlight protocol: https://github.com/moonlight-stream
- Previous Wolf work: `design/2025-09-23-wolf-streaming-architecture.md` (if exists)

## Conclusion

Successfully transitioned Live Stream to work with Wolf lobbies mode while maintaining backward compatibility with apps mode. Users can now enjoy multi-user streaming with PIN protection, and the codebase is cleaner without the keepalive hack. The frontend automatically adapts based on the presence of `wolfLobbyId`, making the transition transparent to users.
